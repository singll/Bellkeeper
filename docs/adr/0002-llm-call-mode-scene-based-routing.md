# 内部 LLM 调用按场景分流：批处理走队列，交互式走直接

内部 LLM 调用有三条路径（直接/队列/外部透传），AskService 运行时条件分支导致行为不可预测。改为编译时确定：批处理（Classify/PKB/RuleOptimizer/DailyReport/KnowledgeScanner）走 llm_jobs 队列，交互式（AskService）走 llmclient 直接调用。场景→模式映射写入开发文档，不再运行时判断。
