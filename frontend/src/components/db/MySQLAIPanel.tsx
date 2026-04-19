import { useState, useRef, useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { mysqlApi, type MySQLAIResponse } from '@/api/mysql'
import { aiApi } from '@/api/ai'
import { useQuery } from '@tanstack/react-query'
import { cn } from '@/lib/utils'
import { Bot, Send, X, Sparkles, AlertTriangle, Info, Loader2, ChevronDown, ChevronUp, Lightbulb } from 'lucide-react'

interface Message {
  id: string
  role: 'user' | 'ai'
  content: string
  suggestions?: string[]
  severity?: string
  provider?: string
  model?: string
  timestamp: Date
  loading?: boolean
}

const QUICK_QUESTIONS = [
  'Is my MySQL healthy right now?',
  'Why is buffer pool hit rate low?',
  'Are there any slow queries I should worry about?',
  'How can I optimize my connection usage?',
  'What indexes should I add?',
  'Analyze my query performance',
]

export function MySQLAIPanel() {
  const [isOpen, setIsOpen] = useState(false)
  const [isMinimized, setIsMinimized] = useState(false)
  const [input, setInput] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  // Check if AI is enabled
  const { data: aiConfig } = useQuery({
    queryKey: ['ai', 'config'],
    queryFn: aiApi.config,
    retry: 1,
    staleTime: 60000,
  })

  const aiEnabled = aiConfig?.enabled ?? false

  const askMutation = useMutation({
    mutationFn: (question: string) => mysqlApi.aiAsk(question),
    onSuccess: (data: MySQLAIResponse) => {
      setMessages(prev => prev.map(m =>
        m.loading ? {
          ...m,
          content: data.answer,
          suggestions: data.suggestions,
          severity: data.severity,
          provider: data.provider,
          model: data.model,
          loading: false,
        } : m
      ))
    },
    onError: (error: Error) => {
      setMessages(prev => prev.map(m =>
        m.loading ? {
          ...m,
          content: `Error: ${error.message}`,
          severity: 'critical',
          loading: false,
        } : m
      ))
    },
  })

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages])

  useEffect(() => {
    if (isOpen && !isMinimized && inputRef.current) {
      inputRef.current.focus()
    }
  }, [isOpen, isMinimized])

  function handleSend(question?: string) {
    const q = question || input.trim()
    if (!q) return
    setInput('')

    const userMsg: Message = {
      id: `user-${Date.now()}`,
      role: 'user',
      content: q,
      timestamp: new Date(),
    }

    const aiMsg: Message = {
      id: `ai-${Date.now()}`,
      role: 'ai',
      content: '',
      timestamp: new Date(),
      loading: true,
    }

    setMessages(prev => [...prev, userMsg, aiMsg])
    askMutation.mutate(q)
  }

  function handleAutoAnalyze() {
    const aiMsg: Message = {
      id: `ai-${Date.now()}`,
      role: 'ai',
      content: '',
      timestamp: new Date(),
      loading: true,
    }
    setMessages(prev => [...prev, {
      id: `user-${Date.now()}`,
      role: 'user' as const,
      content: '🔍 Auto-analyze my MySQL server',
      timestamp: new Date(),
    }, aiMsg])
    askMutation.mutate('')
  }

  if (!aiEnabled) {
    return (
      <div className="fixed bottom-6 right-6 z-50">
        <button
          onClick={() => setIsOpen(!isOpen)}
          className="group flex h-14 w-14 items-center justify-center rounded-full bg-slate-400 text-white shadow-lg transition-all hover:bg-slate-500"
          title="AI not configured"
        >
          <Bot className="h-6 w-6" />
        </button>
        {isOpen && (
          <div className="absolute bottom-16 right-0 w-80 rounded-xl border border-slate-200 bg-white p-5 shadow-2xl dark:border-slate-700 dark:bg-slate-900">
            <div className="flex items-center gap-2 text-slate-500">
              <Info className="h-5 w-5" />
              <p className="text-sm font-medium">AI Not Configured</p>
            </div>
            <p className="mt-2 text-xs text-slate-400">
              Enable AI in Settings → AI to ask questions about your MySQL server. Supports OpenAI, Anthropic, Gemini, Ollama, and custom providers.
            </p>
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="fixed bottom-6 right-6 z-50">
      {/* Floating button */}
      {!isOpen && (
        <button
          onClick={() => setIsOpen(true)}
          className="group flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-blue-600 to-indigo-600 text-white shadow-lg shadow-blue-200 transition-all hover:shadow-xl hover:shadow-blue-300 hover:scale-105 dark:shadow-blue-900/30 dark:hover:shadow-blue-800/40"
          title="Ask AI about MySQL"
        >
          <Sparkles className="h-6 w-6 transition-transform group-hover:rotate-12" />
        </button>
      )}

      {/* Chat panel */}
      {isOpen && (
        <div className={cn(
          'flex flex-col rounded-2xl border border-slate-200 bg-white shadow-2xl transition-all dark:border-slate-700 dark:bg-slate-900',
          isMinimized ? 'w-80 h-14' : 'w-[420px] h-[560px]'
        )}>
          {/* Header */}
          <div className="flex items-center justify-between rounded-t-2xl border-b border-slate-100 bg-gradient-to-r from-blue-600 to-indigo-600 px-4 py-3 dark:border-slate-700">
            <div className="flex items-center gap-2 text-white">
              <Bot className="h-5 w-5" />
              <span className="text-sm font-semibold">MySQL AI Assistant</span>
            </div>
            <div className="flex items-center gap-1">
              <button onClick={() => setIsMinimized(!isMinimized)} className="rounded p-1 text-white/70 hover:bg-white/20 hover:text-white transition-colors">
                {isMinimized ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
              </button>
              <button onClick={() => { setIsOpen(false); setIsMinimized(false) }} className="rounded p-1 text-white/70 hover:bg-white/20 hover:text-white transition-colors">
                <X className="h-4 w-4" />
              </button>
            </div>
          </div>

          {!isMinimized && (
            <>
              {/* Messages */}
              <div ref={scrollRef} className="flex-1 overflow-y-auto p-4 space-y-4">
                {messages.length === 0 && (
                  <div className="space-y-4">
                    <div className="text-center py-4">
                      <Sparkles className="h-8 w-8 mx-auto text-blue-500 mb-2" />
                      <p className="text-sm font-medium text-slate-700 dark:text-slate-300">Ask me anything about your MySQL server</p>
                      <p className="text-xs text-slate-400 mt-1">I can analyze metrics, suggest optimizations, and diagnose issues</p>
                    </div>

                    {/* Auto-analyze button */}
                    <button
                      onClick={handleAutoAnalyze}
                      className="w-full rounded-lg border-2 border-dashed border-blue-200 bg-blue-50/50 p-3 text-left transition-all hover:border-blue-400 hover:bg-blue-50 dark:border-blue-800 dark:bg-blue-950/30 dark:hover:border-blue-600"
                    >
                      <div className="flex items-center gap-2">
                        <Sparkles className="h-4 w-4 text-blue-500" />
                        <span className="text-sm font-medium text-blue-700 dark:text-blue-300">Auto-Analyze My MySQL</span>
                      </div>
                      <p className="mt-1 text-xs text-blue-500/70">Get a full health assessment with recommendations</p>
                    </button>

                    {/* Quick questions */}
                    <div>
                      <p className="text-xs font-medium text-slate-400 mb-2 uppercase tracking-wider">Quick Questions</p>
                      <div className="space-y-1.5">
                        {QUICK_QUESTIONS.map((q, i) => (
                          <button
                            key={i}
                            onClick={() => handleSend(q)}
                            className="block w-full rounded-lg border border-slate-100 bg-slate-50/50 px-3 py-2 text-left text-xs text-slate-600 transition-all hover:border-slate-300 hover:bg-slate-100 dark:border-slate-800 dark:bg-slate-800/30 dark:text-slate-400 dark:hover:border-slate-600 dark:hover:bg-slate-800"
                          >
                            {q}
                          </button>
                        ))}
                      </div>
                    </div>
                  </div>
                )}

                {messages.map(msg => (
                  <div key={msg.id} className={cn('flex', msg.role === 'user' ? 'justify-end' : 'justify-start')}>
                    <div className={cn(
                      'max-w-[85%] rounded-2xl px-4 py-2.5 text-sm',
                      msg.role === 'user'
                        ? 'bg-blue-600 text-white rounded-br-md'
                        : 'bg-slate-100 text-slate-800 dark:bg-slate-800 dark:text-slate-200 rounded-bl-md'
                    )}>
                      {msg.loading ? (
                        <div className="flex items-center gap-2 text-slate-400">
                          <Loader2 className="h-4 w-4 animate-spin" />
                          <span className="text-xs">Analyzing your MySQL metrics...</span>
                        </div>
                      ) : (
                        <>
                          {msg.severity && msg.role === 'ai' && (
                            <div className={cn(
                              'flex items-center gap-1.5 mb-2 text-xs font-medium',
                              msg.severity === 'critical' ? 'text-red-600' : msg.severity === 'warning' ? 'text-amber-600' : 'text-blue-600'
                            )}>
                              {msg.severity === 'critical' ? <AlertTriangle className="h-3.5 w-3.5" /> : msg.severity === 'warning' ? <AlertTriangle className="h-3.5 w-3.5" /> : <Info className="h-3.5 w-3.5" />}
                              {msg.severity === 'critical' ? 'Critical Issue' : msg.severity === 'warning' ? 'Warning' : 'Analysis'}
                            </div>
                          )}
                          <div className="whitespace-pre-wrap text-[13px] leading-relaxed">{msg.content}</div>
                          {msg.suggestions && msg.suggestions.length > 0 && (
                            <div className="mt-3 border-t border-slate-200 dark:border-slate-700 pt-2">
                              <div className="flex items-center gap-1.5 text-xs font-medium text-slate-500 mb-1.5">
                                <Lightbulb className="h-3.5 w-3.5" />
                                Suggestions
                              </div>
                              <ul className="space-y-1">
                                {msg.suggestions.map((s, i) => (
                                  <li key={i} className="text-xs text-slate-600 dark:text-slate-400 flex items-start gap-1.5">
                                    <span className="text-blue-500 font-bold mt-0.5 shrink-0">→</span>
                                    {s}
                                  </li>
                                ))}
                              </ul>
                            </div>
                          )}
                          {msg.provider && (
                            <p className="mt-2 text-[10px] text-slate-400">{msg.provider}/{msg.model}</p>
                          )}
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>

              {/* Input */}
              <div className="border-t border-slate-100 p-3 dark:border-slate-700">
                <div className="flex items-center gap-2">
                  <input
                    ref={inputRef}
                    type="text"
                    value={input}
                    onChange={e => setInput(e.target.value)}
                    onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend() } }}
                    placeholder="Ask about your MySQL server..."
                    className="flex-1 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-700 placeholder:text-slate-400 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-400/20 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200 dark:placeholder:text-slate-500"
                    disabled={askMutation.isPending}
                  />
                  <button
                    onClick={() => handleSend()}
                    disabled={!input.trim() || askMutation.isPending}
                    className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-blue-600 text-white transition-all hover:bg-blue-700 disabled:opacity-40 disabled:cursor-not-allowed"
                  >
                    <Send className="h-4 w-4" />
                  </button>
                </div>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}
