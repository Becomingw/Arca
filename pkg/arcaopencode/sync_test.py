import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import MagicMock, patch

import sync


class OpenCodeSyncTests(unittest.TestCase):
    def test_import_and_refresh_preserve_local_options(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            config_root = home / "config"
            destination = config_root / "opencode" / "opencode.json"
            destination.parent.mkdir(parents=True)
            original = {
                "provider": {
                    "other": {"npm": "configured-locally"},
                    "aether": {
                        "headers": {"Authorization": "stale", "X-Trace": "local"},
                        "options": {"headers": {"API-Key": "stale"}},
                        "models": {"revoked": {"name": "No longer allowed"}},
                    },
                },
                "small_model": "aether/revoked",
            }
            destination.write_text(json.dumps(original))
            gateway = {
                "provider": {
                    "aether": {
                        "npm": "@ai-sdk/openai-compatible",
                        "name": "Arca",
                        "options": {"baseURL": "https://arca.example/v1"},
                        "models": {"allowed": {"name": "Allowed", "id": "allowed"}},
                    },
                },
            }
            response = MagicMock()
            response.status = 200
            response.read.return_value = json.dumps(gateway).encode()
            opener = MagicMock()
            opener.open.return_value.__enter__.return_value = response
            with (
                patch("sync.Path.home", return_value=home),
                patch.dict("sync.os.environ", {"XDG_CONFIG_HOME": str(config_root)}),
                patch("sync.sys.stdin.isatty", return_value=True),
                patch("sync.getpass.getpass", return_value="fixture-key"),
                patch("sync.urllib.request.build_opener", return_value=opener),
            ):
                with patch("sync.sys.argv", ["opencode-sync.py", "--gateway", "https://arca.example"]):
                    sync.main()
                with patch("sync.sys.argv", ["opencode-sync.py"]):
                    sync.main()
            updated = json.loads(destination.read_text())
            self.assertEqual(updated["provider"]["other"], original["provider"]["other"])
            self.assertNotIn("small_model", updated)
            managed = updated["provider"]["aether"]
            self.assertEqual(managed["models"], gateway["provider"]["aether"]["models"])
            self.assertEqual(managed["headers"], {"X-Trace": "local"})
            self.assertEqual(managed["options"]["headers"], {})
            self.assertEqual(managed["options"]["apiKey"], "fixture-key")
            self.assertEqual(destination.stat().st_mode & 0o777, 0o600)
            self.assertEqual((home / ".aether" / "opencode.json").stat().st_mode & 0o777, 0o600)
            self.assertEqual(len(list(destination.parent.glob("*.bak.*"))), 1)
            request = opener.open.call_args.args[0]
            self.assertEqual(request.full_url, "https://arca.example/api/opencode/config")
            self.assertEqual(request.get_header("Authorization"), "Bearer fixture-key")

    def test_redirect_cannot_pass_key_to_another_host(self):
        with self.assertRaises(sync.SyncError):
            sync.NoRedirect().redirect_request(None, None, 307, None, {}, "https://elsewhere.example")


if __name__ == "__main__":
    unittest.main()
