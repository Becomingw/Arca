const responsesModels = ["re:(?i)^(gpt-|grok-|muse-)"];
const messagesModels = ["re:(?i)^(claude-|minimax-|qwen)"];
const geminiModels = ["re:(?i)^gemini-"];
const chatModels = ["re:(?i)^(glm-|kimi-|deepseek-|mimo-|hy[0-9]|space-|longcat-|step-|beam-|cat-)"];
const inference = "/inference/go/v1";
const goQuotaPath = "/zen/go/v1/usage";

export const meta = {
  apiVersion: 1,
  key: "opencode-go",
  name: "OpenCode Go",
  author: { name: "Arca" },
  description: {
    en: "OpenCode Go model routing and per-key usage windows",
    zh: "OpenCode Go 模型路由与密钥用量窗口",
    "zh-TW": "OpenCode Go 模型路由與金鑰用量視窗",
  },
  version: "1.0.0",
  baseUrl: "https://opencode.ai",
  relay: [
    {
      incoming_path: "/v1/chat/completions",
      upstream_path: inference + "/responses",
      models: responsesModels,
      converter: "openai_chat_completions_to_openai_responses",
    },
    {
      incoming_path: "/v1/chat/completions",
      upstream_path: inference + "/messages",
      models: messagesModels,
      converter: "openai_chat_completions_to_anthropic_messages",
    },
    {
      incoming_path: "/v1/chat/completions",
      upstream_path: inference + "/models/{model}:generateContent",
      models: geminiModels,
      converter: "openai_chat_completions_to_gemini_generate_content",
      auth: geminiAuth(),
    },
    { incoming_path: "/v1/chat/completions", upstream_path: inference + "/chat/completions" },
    { incoming_path: "/v1/responses", upstream_path: inference + "/responses", models: responsesModels },
    {
      incoming_path: "/v1/responses",
      upstream_path: inference + "/chat/completions",
      models: chatModels,
      converter: "openai_responses_to_openai_chat_completions",
    },
    {
      incoming_path: "/v1/responses",
      upstream_path: inference + "/models/{model}:generateContent",
      models: geminiModels,
      converter: "openai_responses_to_gemini_generate_content",
      auth: geminiAuth(),
    },
    { incoming_path: "/v1/messages", upstream_path: inference + "/messages", models: messagesModels },
    { incoming_path: "/v1/messages", upstream_path: inference + "/responses", models: responsesModels, converter: "claude_messages_to_openai_responses" },
    {
      incoming_path: "/v1/messages",
      upstream_path: inference + "/chat/completions",
      models: chatModels,
      converter: "anthropic_messages_to_openai_chat_completions",
    },
    {
      incoming_path: "/v1beta/models/{model}:generateContent",
      upstream_path: inference + "/models/{model}:generateContent",
      models: geminiModels,
      auth: geminiAuth(),
    },
    {
      incoming_path: "/v1beta/models/{model}:generateContent",
      upstream_path: inference + "/responses",
      models: responsesModels,
      converter: "gemini_generate_content_to_openai_responses",
    },
    {
      incoming_path: "/v1beta/models/{model}:generateContent",
      upstream_path: inference + "/chat/completions",
      models: chatModels,
      converter: "gemini_generate_content_to_openai_chat_completions",
    },
    { incoming_path: "/v1/models", upstream_path: inference + "/models" },
  ],
};

function geminiAuth() {
  return { type: "header", name: "x-goog-api-key", value: "{api_key}" };
}

const windowLabels = {
  rolling: { en: "Rolling five hours", zh: "滚动五小时", "zh-TW": "滾動五小時" },
  weekly: { en: "Weekly", zh: "每周", "zh-TW": "每週" },
  monthly: { en: "Monthly", zh: "每月", "zh-TW": "每月" },
};
const quotaExhausted = { en: "OpenCode Go quota exhausted", zh: "OpenCode Go 额度已耗尽", "zh-TW": "OpenCode Go 額度已耗盡" };
const keyRejected = { en: "OpenCode Go API key rejected", zh: "OpenCode Go 服务拒绝此密钥", "zh-TW": "OpenCode Go 服務拒絕此金鑰" };
const noSubscription = { en: "OpenCode Go subscription unavailable", zh: "OpenCode Go 订阅不可用", "zh-TW": "OpenCode Go 訂閱無法使用" };
const quotaRestored = { en: "OpenCode Go quota restored", zh: "OpenCode Go 额度已恢复", "zh-TW": "OpenCode Go 額度已恢復" };

export function buildBalanceRequest(ctx) {
  const origin = /^https:\/\/[A-Za-z0-9.:[\]-]+(?=\/|$)/.exec(ctx.baseUrl);
  if (!origin || !ctx.apiKey) {
    throw new Error("OpenCode Go quota requires an HTTPS channel URL and an API key");
  }
  return {
    url: origin[0] + goQuotaPath,
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" },
  };
}

export function parseBalance(_ctx, response) {
  const statusError = usageError(response.status);
  if (statusError) return { error: statusError.en };
  const usage = parseUsage(response.body);
  if (!usage) return { error: "OpenCode Go quota response is invalid" };
  return { windows: usage.windows };
}

export function scheduled(ctx) {
  const response = utils.fetch(buildBalanceRequest(ctx));
  const statusError = usageError(response.status);
  if (statusError) {
    if (ctx.keyStatus === "enabled" && (statusError === keyRejected || statusError === noSubscription)) {
      return { next: 300, keyStatus: "auto_disabled", notify: statusError };
    }
    return { next: 300 };
  }
  const usage = parseUsage(response.body);
  if (!usage) return { next: 300 };
  const result = { next: 120, balance: { windows: usage.windows } };
  if (usage.exhausted) {
    result.next = Math.max(60, usage.latestReset - Math.floor(Date.now() / 1000));
    if (ctx.keyStatus === "enabled") {
      result.keyStatus = "auto_disabled";
      result.notify = quotaExhausted;
    }
  } else if (
    ctx.keyStatus === "auto_disabled" &&
    usage.confirmedHealthy &&
    [quotaExhausted.en, keyRejected.en, noSubscription.en].includes(ctx.disabledReason)
  ) {
    result.keyStatus = "enabled";
    result.notify = quotaRestored;
  }
  return result;
}

function usageError(status) {
  switch (status) {
    case 200:
      return null;
    case 401:
      return keyRejected;
    case 403:
      return noSubscription;
    case 429:
      return { en: "OpenCode Go quota query rate limited" };
    default:
      return { en: "OpenCode Go quota query failed" };
  }
}

function parseUsage(input) {
  if (typeof input === "string") {
    try {
      input = JSON.parse(input);
    } catch {
      return null;
    }
  }
  if (!input || typeof input !== "object" || Array.isArray(input)) return null;
  const quota = input.usage ?? input;
  const result = { windows: [], exhausted: false, latestReset: 0, confirmedHealthy: true };
  const now = Math.floor(Date.now() / 1000);
  for (const [id, name] of Object.entries(windowLabels)) {
    const window = quota[id];
    if (!window || typeof window !== "object" || Array.isArray(window)) {
      result.confirmedHealthy = false;
      continue;
    }
    const rawPercent = window.percent ?? window.usedPercent ?? window.used_percent;
    const percent =
      typeof rawPercent === "number"
        ? rawPercent
        : typeof rawPercent === "string" && /^\d+(?:\.\d+)?%?$/.test(rawPercent.trim())
          ? Number(rawPercent.trim().replace(/%$/, ""))
          : NaN;
    const validPercent = Number.isFinite(percent) && percent >= 0;
    const state =
      typeof window.status === "string"
        ? window.status
            .trim()
            .toLowerCase()
            .replace(/[\s_]+/g, "-")
        : "";
    const limited = ["rate-limited", "limited", "exhausted", "blocked", "exceeded"].includes(state) || percent >= 100;
    const reset = readReset(window.resetsAt ?? window.resets_at ?? window.reset_at);
    if (!validPercent && !limited) {
      result.confirmedHealthy = false;
      continue;
    }
    if (limited && (!reset || reset > now)) {
      result.exhausted = true;
      result.latestReset = Math.max(result.latestReset, reset);
    }
    if (limited || (state && state !== "ok")) result.confirmedHealthy = false;
    const output = { name, used: validPercent ? Math.min(100, percent) : 100, limit: 100, unit: "%" };
    if (reset > now && (percent > 0 || limited)) output.resetAt = reset;
    result.windows.push(output);
  }
  return result.windows.length ? result : null;
}

function readReset(value) {
  if (typeof value === "string" && /^\d{4}-\d\d-\d\dT.*(?:Z|[+-]\d\d:\d\d)$/i.test(value.trim())) {
    value = Date.parse(value.trim()) / 1000;
  } else if (typeof value === "number" && value > 1e12) {
    value /= 1000;
  }
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
}
