# RSS 与提取链路

> 本目录定义 Bellkeeper 在 RSS 入口与正文提取策略中的职责。

---

## 定位

Bellkeeper 不负责替代 RSSHub 或 n8n，而是负责：

- 维护 RSS 订阅配置资产
- 为工作流提供统一 RSS 管理 API
- 定义正文提取与文件治理策略
- 作为 RSS → 文件入库链路中的治理中心

---

## 文档

1. [EXTRACTION-PIPELINE.md](EXTRACTION-PIPELINE.md)
   - RSSHub、Trafilatura、Firecrawl 在新链路中的分工

---

## 一句话定义

**RSS 模块提供来源入口，documents 模块提供文件治理，二者共同构成 Bellkeeper 的“采集到文件”主链支撑。**
