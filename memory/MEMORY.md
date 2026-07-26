# Bellkeeper MEMORY

> 最后更新: 2026-07-26 · **1.0 已 GA，稳定运行**
>
> 本文件是简要进度快照。**当前状态权威见 [doc/STATUS.md](../doc/STATUS.md)**，架构见 [doc/ARCHITECTURE.md](../doc/ARCHITECTURE.md)，历史演进见 [doc/TIMELINE.md](../doc/TIMELINE.md)。

## 当前状态

- **1.0 里程碑 M1–M6 全部完成**（M1 事件总线 / M2 LLM 独立化 / M3 KB 链路事件化 / M4 日志补齐 / M5 前端收敛 / M6 可观测性）。tag `v1.0.0` 已推送。
- **部署形态**：keeper(.230) 应用层 ↔ silkdata(.231) 数据层分离（2026-07-25）；观测栈 Prometheus+Loki+Grafana@silkdata（2026-07-26）。
- **PKB**：原子知识网 + 知识骨架/双库 + 自动闭环调度全部落地；存量 raw 分批跑为持续数据运营。
- **爬取**：DequeueFair 公平调度 + 域名冷却/健康度 + recrawl 去重；firecrawl fetch 引擎优先降流量。

## 剩余待办

见 [doc/ROADMAP.md](../doc/ROADMAP.md)：silkdata 割接收尾、PKB 运营、爬虫新源验收、LLM cache 监控等（无 P0 阻塞）。待决：openclash 省流量 VPS（观察）。txhk 已由 Conduit 迁 **tuwunel**（2026-07-02 完成）、**new-api 容器已退役**。
