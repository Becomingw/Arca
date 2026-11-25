#!/usr/bin/env python3
import argparse
import copy
import fcntl
import getpass
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request


class SyncError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise SyncError(message)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, 'Duplicate JSON keys are not supported; nothing overwritten.')
        result[key] = value
    return result


def invalid_constant(value):
    raise SyncError('Non-standard JSON is not supported; nothing overwritten.')


def parse_json(raw):
    try:
        value = json.loads(raw, object_pairs_hook=unique_object, parse_constant=invalid_constant)
    except (ValueError, UnicodeError):
        raise SyncError('Invalid JSON (JSONC is not supported); nothing overwritten.') from None
    require(isinstance(value, dict), 'Expected a JSON object; nothing overwritten.')
    return value


def merge_headers(previous, incoming, merged):
    if 'headers' not in previous and 'headers' not in incoming:
        return
    credential_headers = {'authorization', 'proxy-authorization', 'x-api-key', 'api-key',
                          'api_key', 'x-goog-api-key', 'x-auth-token', 'x-access-token'}
    headers = {}
    for source in (previous, incoming):
        values = source.get('headers', {})
        require(isinstance(values, dict), 'Arca headers must be JSON objects; nothing overwritten.')
        headers.update({name: value for name, value in values.items()
                        if name.strip().lower() not in credential_headers})
    merged['headers'] = headers


def private_file(path):
    info = path.lstat()
    require(stat.S_ISREG(info.st_mode) and info.st_uid == os.getuid()
            and not info.st_mode & 0o077,
            'Sync credentials must be a private, owned regular file (chmod 600).')
    return path.read_bytes()


def private_directory(path):
    require(not path.is_symlink(), 'Refusing a symlinked configuration directory.')
    path.mkdir(parents=True, exist_ok=True, mode=0o700)
    require(path.stat().st_uid == os.getuid(), 'Configuration directory must be owned by this user.')


def atomic_write(path, content, mode=0o600):
    require(not path.is_symlink(), 'Refusing a symlinked configuration file.')
    fd, temporary = tempfile.mkstemp(prefix='.' + path.name + '.', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as output:
            os.fchmod(output.fileno(), mode)
            output.write(content)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise SyncError('Gateway redirect refused; credentials were not forwarded.')


def main():
    parser = argparse.ArgumentParser(description='Sync OpenCode models and configuration from Arca')
    parser.add_argument('--gateway', help='HTTPS gateway address, without /v1')
    arguments = parser.parse_args()
    installing = arguments.gateway is not None
    state_dir = Path.home() / '.aether'
    state_path = state_dir / 'opencode.json'
    script_path = state_dir / 'opencode-sync.py'
    require(not state_path.is_symlink() and not script_path.is_symlink(),
            'Refusing symlinked sync credentials or script.')
    require(all(not item.exists() or item.is_file() for item in (state_path, script_path)),
            'Sync credentials and script must be regular files.')
    private_directory(state_dir)
    os.chmod(state_dir, 0o700)
    lock_path = state_dir / 'opencode-sync.lock'
    fd = os.open(lock_path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    lock = os.fdopen(fd, 'w')
    os.fchmod(lock.fileno(), 0o600)
    # Read credentials under the lock so a concurrent reinstall cannot leave a stale key.
    fcntl.flock(lock, fcntl.LOCK_EX)
    if installing:
        config_root = Path(os.environ.get('XDG_CONFIG_HOME') or str(Path.home() / '.config'))
        require(config_root.is_absolute(), 'XDG_CONFIG_HOME must be an absolute path.')
        require(sys.stdin.isatty(), 'Run initial setup in a terminal.')
        credentials = {'base_url': arguments.gateway,
                       'api_key': getpass.getpass('Inference API key: ').strip(),
                       'config_path': str(config_root / 'opencode' / 'opencode.json')}
    else:
        require(not state_dir.is_symlink(), 'Refusing a symlinked credential directory.')
        credentials = parse_json(private_file(state_path))
    base = credentials.get('base_url')
    key = credentials.get('api_key')
    destination = credentials.get('config_path')
    require(all(isinstance(item, str) and item for item in (base, key, destination)),
            'Invalid local sync credentials; reinstall from the gateway.')
    require(not any(char in key for char in '\r\n'), 'Invalid API key.')
    require(base == base.strip() and not any(char in base for char in '\r\n'), 'Invalid gateway URL.')
    base = base.rstrip('/')
    if base.endswith('/v1'):
        base = base[:-3]
    url = urllib.parse.urlsplit(base)
    require(url.hostname and not url.username and not url.password and not url.query and not url.fragment
            and (url.scheme == 'https' or (url.scheme == 'http' and url.hostname in ('localhost', '127.0.0.1', '::1'))),
            'Gateway URL must use HTTPS (HTTP is allowed only for loopback), without credentials or query.')
    path = Path(destination)
    require(path.is_absolute() and path.name == 'opencode.json', 'Invalid sync destination.')
    with lock:
        require(not path.parent.is_symlink() and not path.is_symlink(),
                'Refusing a symlinked OpenCode configuration.')
        require(not path.exists() or path.is_file(),
                'OpenCode configuration must be a regular JSON file; nothing overwritten.')
        require(not os.path.lexists(path.with_suffix('.jsonc')),
                'opencode.jsonc exists; JSONC merging is not supported. No configuration overwritten.')
        original = path.read_bytes() if path.exists() else None
        data = parse_json(original) if original is not None else {}
        providers = data.setdefault('provider', {})
        require(isinstance(providers, dict), 'provider must be a JSON object; nothing overwritten.')
        previous = providers.get('aether', {})
        require(isinstance(previous, dict) and isinstance(previous.get('options', {}), dict),
                'provider.aether/options must be JSON objects; nothing overwritten.')

        request = urllib.request.Request(base + '/api/opencode/config',
                                         headers={'Authorization': 'Bearer ' + key, 'Accept': 'application/json'})
        try:
            with urllib.request.build_opener(NoRedirect()).open(request, timeout=30) as response:
                require(response.status == 200, 'Gateway config request failed; nothing overwritten.')
                raw = response.read(8 * 1024 * 1024 + 1)
        except (urllib.error.URLError, TimeoutError, OSError):
            raise SyncError('Gateway config request failed; check access, TLS and API key. Nothing overwritten.') from None
        require(len(raw) <= 8 * 1024 * 1024, 'Gateway config is too large; nothing overwritten.')
        remote = parse_json(raw)
        provider = remote.get('provider', {})
        require(isinstance(provider, dict), 'Invalid gateway provider config; nothing overwritten.')
        incoming = provider.get('aether')
        require(isinstance(incoming, dict) and isinstance(incoming.get('options'), dict)
                and isinstance(incoming.get('models'), dict)
                and incoming.get('npm') == '@ai-sdk/openai-compatible'
                and incoming['options'].get('baseURL') == base + '/v1',
                'Invalid gateway Arca config/baseURL; nothing overwritten.')
        require(all(isinstance(meta, dict) for meta in incoming['models'].values()),
                'Invalid gateway model metadata; nothing overwritten.')
        managed = copy.deepcopy(previous)
        managed.update(npm=incoming['npm'], models=copy.deepcopy(incoming['models']))
        if isinstance(incoming.get('name'), str):
            managed['name'] = incoming['name']
        managed['options'] = {**previous.get('options', {}), 'baseURL': incoming['options']['baseURL'], 'apiKey': key}
        # Explicit credential headers override the SDK apiKey, including after key rotation.
        merge_headers(previous, incoming, managed)
        merge_headers(previous.get('options', {}), incoming['options'], managed['options'])
        providers['aether'] = managed
        cleared_defaults = []
        for field in ('model', 'small_model'):
            selected = data.get(field)
            if isinstance(selected, str) and selected.startswith('aether/') and selected[len('aether/'):] not in managed['models']:
                del data[field]
                cleared_defaults.append(field)
        updated = (json.dumps(data, ensure_ascii=False, indent=2, allow_nan=False) + '\n').encode('utf-8')
        private_directory(path.parent)
        # The gateway owns its catalog; replacing models removes revoked/stale entries.
        if original is not None and original != updated:
            backup_fd, backup = tempfile.mkstemp(prefix=path.name + '.bak.', dir=path.parent)
            with os.fdopen(backup_fd, 'wb') as output:
                os.fchmod(output.fileno(), 0o600)
                output.write(original)
                output.flush()
                os.fsync(output.fileno())
        if original != updated:
            atomic_write(path, updated)
        else:
            os.chmod(path, 0o600)
        if installing:
            atomic_write(state_path, (json.dumps(credentials, indent=2) + '\n').encode('utf-8'))
            atomic_write(script_path, Path(__file__).read_bytes(), mode=0o700)
    if cleared_defaults:
        print('[Arca] Cleared unavailable Arca defaults: ' + ', '.join(cleared_defaults) + '. Choose a model in OpenCode.')
    print('[Arca] OpenCode config synced. Restart OpenCode to load the updated catalog.')


if __name__ == "__main__":
    try:
        main()
    except SyncError as error:
        print('[Arca] ' + str(error), file=sys.stderr)
        sys.exit(1)
    except Exception:
        # Never echo network bodies, credentials, parsed values or exception tracebacks.
        print('[Arca] OpenCode sync failed; check private files and gateway availability.', file=sys.stderr)
        sys.exit(1)
