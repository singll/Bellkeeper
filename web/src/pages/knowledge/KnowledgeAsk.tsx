import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { knowledgeAskApi, knowledgeIndexApi } from '@/api/knowledge'
import { useToast } from '@/components/Toast'
import type { KnowledgeAskReference } from '@/types'

interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  references?: KnowledgeAskReference[]
  timestamp: Date
  isError?: boolean
}

const KnowledgeAsk: Component = () => {
  const toast = useToast()

  // State
  const [question, setQuestion] = createSignal('')
  const [history, setHistory] = createSignal<ChatMessage[]>([])
  const [asking, setAsking] = createSignal(false)

  // Index status
  const [indexStats] = createResource(() => knowledgeIndexApi.getStats())

  // Handle ask
  const handleAsk = async (e: Event) => {
    e.preventDefault()
    if (!question().trim()) return

    const userMsg: ChatMessage = {
      id: `user-${Date.now()}`,
      role: 'user',
      content: question(),
      timestamp: new Date(),
    }

    // 当前问题之前的历史：最近 6 轮（12 条），排除 error 占位
    const priorTurns = history()
      .filter((m) => !m.isError)
      .slice(-12)
      .map((m) => ({ role: m.role, content: m.content }))

    setHistory([...history(), userMsg])
    setQuestion('')
    setAsking(true)

    try {
      const result = await knowledgeAskApi.ask({
        question: userMsg.content,
        top_k: 5,
        history: priorTurns,
      })

      const assistantMsg: ChatMessage = {
        id: `assistant-${Date.now()}`,
        role: 'assistant',
        content: result.data.answer,
        references: result.data.references,
        timestamp: new Date(),
      }

      setHistory([...history(), assistantMsg])
    } catch (err) {
      toast.error('问答失败: ' + (err as Error).message)

      const errorMsg: ChatMessage = {
        id: `error-${Date.now()}`,
        role: 'assistant',
        content: '抱歉，发生了错误。请稍后重试。',
        timestamp: new Date(),
        isError: true,
      }
      setHistory([...history(), errorMsg])
    } finally {
      setAsking(false)
    }
  }

  // Clear history
  const clearHistory = () => {
    setHistory([])
  }

  // Format time
  const formatTime = (date: Date) => {
    return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  }

  return (
    <div class="animate-fade-in h-full flex flex-col">
      {/* Header */}
      <div class="flex items-center justify-between mb-4">
        <div>
          <h1 class="text-2xl font-bold text-white">知识问答</h1>
          <p class="text-sm text-dark-400 mt-1">AI 助手（优先知识库）</p>
        </div>
        <Show when={indexStats()?.data}>
          <div class="text-sm text-dark-400">
            已索引: {indexStats()?.data?.indexed_count || 0} 个文档
            {indexStats()?.data?.is_indexing && <span class="text-yellow-400 ml-2">索引中...</span>}
          </div>
        </Show>
      </div>

      {/* Chat Container */}
      <div class="flex-1 card flex flex-col overflow-hidden">
        {/* Messages */}
        <div class="flex-1 overflow-y-auto p-4 space-y-4">
          <Show
            when={history().length > 0}
            fallback={
              <div class="h-full flex items-center justify-center text-dark-500">
                <div class="text-center">
                  <p class="mb-2">开始提问吧</p>
                  <p class="text-sm">基于知识库中的文档进行回答</p>
                </div>
              </div>
            }
          >
            <For each={history()}>
              {(msg) => (
                <div class={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                  <div
                    class={`max-w-[80%] rounded-lg p-4 ${
                      msg.role === 'user'
                        ? 'bg-primary-600 text-white'
                        : 'bg-dark-700 text-dark-100'
                    }`}
                  >
                    {/* Role badge */}
                    <div class="text-xs opacity-60 mb-1">
                      {msg.role === 'user' ? '你' : 'AI'}
                    </div>

                    {/* Content */}
                    <div class="prose prose-sm max-w-none">
                      <p class="whitespace-pre-wrap">{msg.content}</p>
                    </div>

                    {/* References */}
                    <Show when={msg.role === 'assistant' && msg.references?.length}>
                      <div class="mt-3 pt-3 border-t border-dark-600">
                        <div class="text-xs text-dark-400 mb-2">📎 参考来源:</div>
                        <For each={msg.references}>
                          {(ref) => (
                            <div class="text-xs text-dark-300 mb-1 last:mb-0">
                              <span class="text-primary-400">{ref.title}</span>
                              <Show when={ref.file_path}>
                                <span class="text-dark-500 ml-2">{ref.file_path}</span>
                              </Show>
                              <Show when={ref.snippet}>
                                <div class="text-dark-400 mt-1 line-clamp-2">
                                  "{ref.snippet}"
                                </div>
                              </Show>
                            </div>
                          )}
                        </For>
                      </div>
                    </Show>

                    {/* Time */}
                    <div class="text-xs opacity-40 mt-2 text-right">
                      {formatTime(msg.timestamp)}
                    </div>
                  </div>
                </div>
              )}
            </For>

            {/* Loading indicator */}
            <Show when={asking()}>
              <div class="flex justify-start">
                <div class="bg-dark-700 rounded-lg p-4">
                  <div class="flex items-center gap-2">
                    <div class="loading-spinner" />
                    <span class="text-dark-400">思考中...</span>
                  </div>
                </div>
              </div>
            </Show>
          </Show>
        </div>

        {/* Input Area */}
        <div class="border-t border-dark-700 p-4">
          {/* Input form */}
          <form onSubmit={handleAsk} class="flex gap-3">
            <input
              type="text"
              class="input flex-1"
              placeholder="输入问题..."
              value={question()}
              onInput={(e) => setQuestion(e.currentTarget.value)}
              disabled={asking()}
            />
            <button
              type="submit"
              class="btn btn-primary"
              disabled={asking() || !question().trim()}
            >
              {asking() ? (
                <div class="loading-spinner" />
              ) : (
                <>
                  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
                  </svg>
                  提问
                </>
              )}
            </button>
            <Show when={history().length > 0}>
              <button
                type="button"
                class="btn btn-ghost"
                onClick={clearHistory}
                title="清空对话"
              >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
              </button>
            </Show>
          </form>
        </div>
      </div>
    </div>
  )
}

export default KnowledgeAsk
