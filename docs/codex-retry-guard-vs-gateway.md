# Codex Retry Guard 与 Codex Retry Gateway 对比说明

日期：2026-07-02

本文基于两份代码的当前状态整理。

`codex-retry-gateway` 来源为 `/opt/src/codex-retry-gateway`，当前提交是 `590ab74`，远端仓库是 `https://github.com/nonononull/codex-retry-gateway.git`。

`codex-retry-guard` 来源为 `/opt/src/worktrees/CLIProxyAPI/codex-cpa-plugin-retry-guard-main/plugins/codex-retry-guard`，当前已经作为 CPA 动态插件部署到 `/opt/cliproxyapi-vps/plugins/codex-retry-guard.so`。

## 一句话区别

`Codex Retry Gateway` 是一个独立本地代理。它会接管本机 Codex 当前 provider 的 `base_url`，自己监听 `127.0.0.1:4610`，再把请求转发到真实上游，并在转发过程中做防降智检查、重试、统计、探针和恢复配置。

`Codex Retry Guard` 是 CPA 里的动态插件。它不接管 Codex 本地配置，也不自己当公网或本地代理。请求先进入 CPA，由 CPA 完成认证、路由、协议处理和上游调用，插件只挂在 CPA 的请求、响应、流式响应拦截点上，检查经过 CPA 的结果是否命中防降智规则。

所以两者目标相似，但所在位置不同。Gateway 是一层完整网关，Guard 是 CPA 管道里的一个过滤器。

## 架构差异

| 项目 | Codex Retry Gateway | Codex Retry Guard |
| --- | --- | --- |
| 运行形态 | Node.js 单进程本地网关 | Go `c-shared` 动态库，由 CPA 插件系统加载 |
| 接入方式 | 修改 Codex `config.toml` 的 provider `base_url` | 在 CPA 插件商店安装，CPA 加载 `.so` |
| 请求入口 | Codex 直接请求本地 gateway | Codex 或 SDK 先请求 CPA，再进入插件拦截点 |
| 上游调用 | Gateway 自己 `fetch` 上游 | CPA 负责上游调用，插件必要时通过 host callback 触发重试 |
| 认证来源 | 复用本机 Codex `auth.json` 或 provider 配置 | 复用 CPA 已有认证、API Key、模型路由配置 |
| 配置位置 | `~/.codex-retry-gateway/config/config.json` 与管理页 | CPA `plugins.configs.codex-retry-guard` 与插件管理面板 |
| 管理页面 | Gateway 自带完整 UI | CPA 管理页中显示插件状态页和配置字段 |
| 恢复能力 | 可以恢复本机 Codex 原 `config.toml` | 不需要恢复 Codex 本地配置，因为插件不改它 |

## 已经从原项目采用的功能

| 原项目功能 | 插件当前状态 | 说明 |
| --- | --- | --- |
| 检查 `reasoning_tokens` | 已采用 | 非流式和流式响应都会提取 `reasoning_tokens`。 |
| 默认异常集合 `516 / 1034 / 1552` | 已采用 | 默认 `reasoning_equals` 与原项目一致。 |
| 支持 root 与 `/v1` 路径 | 已采用 | 默认覆盖 `/responses`、`/chat/completions`、`/v1/responses`、`/v1/chat/completions`。 |
| 非流式命中后返回 `502` | 已采用 | 命中且重试耗尽时返回配置的 `non_stream_status_code`，默认 `502`。 |
| 流式 strict 缓冲后再判断 | 已采用 | 插件在流式初始化阶段通过 `X-CPA-Buffer-Stream: 1` 要求 CPA 缓冲，避免先吐出半截流再发现命中。 |
| 命中后内部重试 | 已采用 | 非流式通过 `HostModelExecute` 重试，流式通过 `HostModelExecuteStream`、`HostModelStreamRead`、`HostModelStreamClose` 重试。 |
| `guard_retry_attempts` | 已采用 | 默认 3，可在 CPA 插件配置里调整。 |
| 按模型过滤 | 插件新增 | 原 Gateway 主要做模型家族观测和探针目标家族，不是防降智白名单。插件现在新增 `models`，空列表表示所有模型，填入模型名后只检查这些模型。 |
| 流式与非流式独立开关 | 已采用 | `intercept_streaming` 与 `intercept_non_streaming` 已暴露到 CPA 配置字段。 |
| 命中日志 | 已采用 | 插件记录 request、inspect、match 日志，并在状态页展示最近日志。 |
| 状态统计 | 部分采用 | 已有 inspected、matched、blocked、日志条数、最后请求画像。原项目更丰富。 |
| 管理页配置 | 部分采用 | 已把 9 个配置字段暴露到 CPA 插件管理。界面体验仍依赖 CPA 通用配置表单。 |

## 没有采用原项目的功能

| 原项目功能 | 插件未采用原因 |
| --- | --- |
| 修改 Codex 本地 `config.toml` | 插件安装在 CPA 里，不应该碰用户本机 Codex 配置。用户只需要让客户端接 CPA。 |
| 读取或恢复本机 `auth.json` | CPA 已经有自己的认证文件、API Key、OAuth 和模型路由系统，插件不重复实现认证层。 |
| 独立监听 `127.0.0.1:4610` | CPA 本身就是网关，插件不再额外开 HTTP 代理端口。 |
| 一键安装、启动、停止、恢复脚本 | 这些是本地 gateway 产品的生命周期能力。插件生命周期由 CPA 插件商店和 CPA 服务管理。 |
| Gateway 完整 UI | CPA 已有管理中心。插件只提供状态页和配置字段，不复制整套 gateway UI。 |
| 模型家族一致性统计 | 当前插件没有实现 `local_config_model`、`upstream_model_counts`、`stream_model_counts`、声明一致率、模型漂移等统计。 |
| 主动探针 | 当前插件没有实现长上下文、图片输入、结构一致性、身份一致性、知识截止日期等主动探针。 |
| UI 一键恢复 Codex 原设置 | 插件没有修改 Codex 原设置，因此没有恢复对象。 |
| Gateway 自己的健康检查路径 | CPA 有服务级健康和管理接口，插件只提供自己的状态接口。 |
| 请求体大小限制与本地代理级限流 | CPA 负责入口层限制，插件只处理 CPA 交给它的拦截事件。 |

## 当前插件还保留的边界

插件和原项目一样，不负责 `Responses` 与 `Chat Completions` 的协议互转。如果上游本身不支持当前请求协议，插件不会把它修成兼容协议。

插件也不能证明“真实底层模型到底是什么”。`reasoning_tokens = 516 / 1034 / 1552` 只是当前经验规则中的高风险信号，命中后可以重试或拦截，但它不是模型鉴定结论。

插件当前的统计默认是进程内存状态。CPA 重启后，插件状态页里的计数和日志会重新开始。这和原 gateway 的“本次启动以来”口径接近，但还没有持久化历史。

## 当前插件可以继续优化的地方

| 优化项 | 建议 |
| --- | --- |
| README 过期说明 | 插件 README 里还写着内部重试未接 host callback，这已经不符合当前代码，应更新。 |
| 状态页来源 | 现在为了绕过公网 `/v0/resource` 路由限制，CPA 主前端里还有一段针对本插件的特殊面板。更稳的方向是修 CPA 的插件资源路由或 Cloudflare 转发，让插件自己的资源页直接 iframe 展示。 |
| 中文化 | 固定标题已中文化，但日志正文仍是 `[request] path=...` 这类工程日志。可以在插件日志输出层直接改成中文，或状态页做安全翻译。 |
| 配置字段体验 | CPA 通用表单对数组字段不够友好。`models`、`reasoning_equals` 和 `endpoints` 最好提供更明确的数组编辑体验，避免用户输入格式错误。 |
| 数组类型兼容 | CPA 通用数组表单可能把数字数组保存成字符串数组。插件已经兼容 `reasoning_equals: ["516", "1034"]` 这类写法，但后续新增数值数组字段时也要按这个规则处理。 |
| `stream_action` 扩展 | 当前可视化枚举只有 `strict_502`。代码里仍有 `disconnect` 分支，可以决定是正式开放，还是删除旧兼容分支。 |
| 指标补齐 | 可以补 `observed_reasoning_counts`、流式/非流式占比、实际拦截率、内部重试次数、最终成功率。 |
| 模型一致性 | 如果 CPA 能提供请求模型、上游声明模型、流式事件模型，可以移植原项目的轻量模型一致性统计。 |
| 主动探针 | 可以做 CPA 版主动探针，但要谨慎。它会主动消耗额度，并且结论只能叫“契约证伪”或“风险样本”，不能叫模型鉴定。 |
| 日志持久化 | 当前日志在内存里，重启丢失。可以落到 CPA 日志系统或插件自己的轻量 ring buffer 文件。 |
| 重启 SIGSEGV | 当前 CPA 重启时旧进程偶尔出现 cgo unload 相关 core dump，虽然新进程能启动，但这需要单独排查。 |
| 发布流程 | 插件商店 registry、zip 包、版本号、变更说明可以规范化，避免线上 `.so` 和 registry 元数据不一致。 |

## 总结

`Codex Retry Gateway` 是完整本地代理产品。它解决的是“本机 Codex 怎么被接到一个可观测、可重试、可恢复的防降智代理上”。

`Codex Retry Guard` 是把其中最核心的防降智逻辑移植进 CPA。它解决的是“所有经过 CPA 的 Codex 请求，能不能在 CPA 内部统一做 reasoning token 检查、重试和拦截”。

当前插件已经覆盖原项目的核心防降智闭环：检查 `reasoning_tokens`、命中 `516 / 1034 / 1552`、按模型可选过滤、流式 strict 缓冲、非流式与流式内部重试、最终 `502` 拦截、基础状态与配置面板。

当前插件没有复制原项目的本地安装恢复体系、完整 UI、模型一致性统计和主动探针。这个取舍是合理的，因为 CPA 已经承担了网关、认证、路由和管理中心职责。下一步更值得做的是把状态页、日志、配置体验和指标持久化做稳，而不是把整个 gateway 原样搬进插件。
