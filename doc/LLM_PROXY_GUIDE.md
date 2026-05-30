# LLM Proxy 配置指南

> Bellkeeper 内置 LLM Proxy，提供 OpenAI 兼容的 API 代理，支持多渠道、速率限制、自动重试、虚拟模型组、粘性路由和熔断器。

## 架构概览

```
调用方（n8n / RAGFlow / Matrix Bot）
  ↓
Bellkeeper LLM Proxy (/api/llm/v1/*)
  ├─ 虚拟模型组解析（pool-chat-free → 真实渠道）
  ├─ 粘性路由（同一任务绑定同一渠道）
  ├─ 熔断器（连续失败自动暂停渠道）
  ├─ 令牌桶限速 (RPM/RPD)
  ├─ 指数退避重试 (429/5xx)
  └─ 多渠道优先级路由
  ↓
上游 LLM API（SiliconFlow / 阿里百炼 / DeepSeek / new-api / Kimi）
```

## 环境变量配置

在 `SilkSpool/hosts/keeper/.env` 中配置 API Key：

```bash
# --- LLM Proxy API Keys ---
LLM_NEWAPI_BASE_URL=https://your-newapi-instance.example.com
LLM_NEWAPI_API_KEY=your-newapi-api-key
LLM_DEEPSEEK_API_KEY=sk-xxxxxxxxxxxxxxxx
LLM_KIMI_API_KEY=sk-xxxxxxxxxxxxxxxx
```

这些变量通过 docker-compose 传入容器，由 `bellkeeper.yaml` 中的 `${VAR}` 引用。

## 注册新 API Key 后如何添加

### 步骤 1: 编辑渠道配置

打开 `Bellkeeper/config/bellkeeper.yaml`，在 `llm_proxy.channels` 列表中添加新渠道：

```yaml
llm_proxy:
  channels:
    # ... 已有渠道 ...

    - name: your-channel-name        # 渠道唯一名称
      base_url: ${YOUR_NEW_API_BASE_URL}  # 使用环境变量引用，或直接写 URL
      api_key: ${YOUR_NEW_API_KEY}        # 使用环境变量引用
      rpm: 10                             # 每分钟请求限制
      rpd: 500                            # 每日请求限制
      priority: 1                         # 优先级 (数值越小越优先)
      models:                             # 该渠道支持的模型列表
        - model-name-1
        - model-name-2
      is_enabled: true                    # 是否启用
```

### 步骤 2: 添加环境变量（如果使用了 `${VAR}` 引用）

**a) 在 `.env` 中添加：**

编辑 `SilkSpool/hosts/keeper/.env`，添加新的 API Key：

```bash
YOUR_NEW_API_BASE_URL=https://api.example.com
YOUR_NEW_API_KEY=sk-xxxxxxxxxxxxxxxx
```

**b) 在 docker-compose 模板中传递：**

编辑 `SilkSpool/bundles/keeper/templates/50-bellkeeper.yaml`，在 bellkeeper 服务的 `environment` 部分添加：

```yaml
- YOUR_NEW_API_BASE_URL=${YOUR_NEW_API_BASE_URL:-}
- YOUR_NEW_API_KEY=${YOUR_NEW_API_KEY:-}
```

### 步骤 3: 重建容器

```bash
# 方式一: 通过 spool.sh (推荐)
./spool.sh bundle keeper service keeper bellkeeper up --force-recreate

# 方式二: 如果 base_url 是直接写的 URL (不用 ${VAR})，
# 且只改了 bellkeeper.yaml，则需要重建以烘焙新配置到镜像
```

## 已配置的渠道

| 渠道名 | API 提供商 | 支持模型 | RPM | RPD | 免费 |
|--------|-----------|---------|-----|-----|------|
| siliconflow-qwen3-8b | SiliconFlow | Qwen/Qwen3-8B | 500 | 50000 | 是 |
| siliconflow-qwen25-7b | SiliconFlow | Qwen/Qwen2.5-7B-Instruct | 500 | 50000 | 是 |
| qwen-plus-direct | 阿里百炼 | qwen3.5-plus, qwen-plus | 500 | 20000 | 否 |
| qwen-flash-direct | 阿里百炼 | qwen3.5-flash, qwen-turbo | 1000 | 50000 | 否 |
| deepseek-direct | DeepSeek | deepseek-chat | 10 | 500 | 否 |
| glm-flash-via-newapi | New API (智谱) | glm-4-flash, GLM-4.7-Flash | 30 | 1500 | 是 |
| kimi-direct | Moonshot (Kimi) | moonshot-v1-128k | 3 | 50 | 否 |

## 虚拟模型组

虚拟模型组允许调用方使用一个虚拟模型名（如 `pool-chat-free`），Proxy 自动在多个真实渠道间做健康感知的智能调度。

### 已配置的模型组

| 组名 | 用途 | 成员渠道 | 粘性 TTL |
|------|------|---------|---------|
| `pool-chat-free` | 分类/解析增强（免费） | Qwen3-8B → Qwen2.5-7B → GLM | 600s |
| `pool-chat-balanced` | QA 问答（付费主力+免费兜底） | qwen3.5-plus → Qwen3-8B → GLM | 300s |
| `pool-summary` | AI 总结（DeepSeek + Qwen 备用） | deepseek-chat → qwen3.5-plus | 900s |

### 使用方式

调用方只需将请求体中的 `model` 字段设为虚拟模型组名：

```json
{
  "model": "pool-chat-free",
  "messages": [{"role": "user", "content": "..."}]
}
```

可选请求头：
- `X-Task-Key: doc_abc123` — 任务级粘性路由 key（同一 key 的请求绑定同一渠道）
- `X-Caller-ID: ragflow-parse` — 调用方标识

## 熔断器

当某个渠道连续失败达到阈值（默认 5 次），会自动进入熔断状态，停止向该渠道发送请求。冷却期（默认 120 秒）过后进入半开状态，放行少量探测请求，成功则恢复。

## 管理接口

| 接口 | 说明 |
|------|------|
| `GET /api/llm/channels/status` | 查看所有渠道状态、限速桶和健康信息 |
| `GET /api/llm/health` | 查看所有渠道健康状态（熔断、成功率） |
| `GET /api/llm/groups/status` | 查看所有虚拟模型组状态和粘性绑定数 |
| `GET /api/llm/stats?hours=24` | 查看使用统计 |
| `GET /api/llm/logs?limit=50` | 查看最近请求日志 |
| `GET /api/llm/rate-limit-events` | 查看限速事件 |
| `DELETE /api/llm/groups/:name/sticky` | 清除指定模型组的所有粘性绑定 |
| `POST /api/llm/channels/:name/reset` | 手动重置指定渠道的熔断状态 |

## 注意事项

- `base_url` 和 `api_key` 支持 `${ENV_VAR}` 语法，运行时会自动从容器环境变量展开
- 如果直接在 YAML 中写死 URL/Key（不用 `${}`），则不需要配置环境变量和 docker-compose 传递
- 同一个模型可以配置多个渠道，Proxy 会按 `priority` 排序，优先使用高优先级渠道
- 当一个渠道达到速率限制或返回错误时，会自动尝试下一个渠道
- 修改 `bellkeeper.yaml` 后需要重建容器（配置文件烘焙在镜像中）
