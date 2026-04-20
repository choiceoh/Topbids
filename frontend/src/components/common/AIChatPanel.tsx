import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useParams } from 'react-router'
import { Loader2, MessageCircle, Send, X } from 'lucide-react'
import Markdown from 'react-markdown'

import { Button } from '@/components/ui/button'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useAIChat, type ChatMessage } from '@/hooks/useAIChat'
import { useCollection } from '@/hooks/useCollections'

const DEFAULT_HINTS = [
  '앱을 만들려면 어떻게 하나요?',
  '현재 어떤 앱들이 있나요?',
  '항목 유형에는 어떤 것들이 있나요?',
]

export default function AIChatPanel() {
  const aiAvailable = useAIAvailable()
  const { appId } = useParams<{ appId?: string }>()
  const location = useLocation()
  const { data: collection } = useCollection(appId)

  const [open, setOpen] = useState(false)
  const [input, setInput] = useState('')
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  const chatMutation = useAIChat()

  const hints = useMemo(() => {
    if (!collection) return DEFAULT_HINTS
    const name = collection.label
    const isView = location.pathname.endsWith(appId ?? '')
    const isDashboard = location.pathname.includes('/dashboard')
    const isSettings = location.pathname.includes('/settings')
    const isProcess = location.pathname.includes('/process')

    if (isDashboard) {
      return [
        `"${name}" 대시보드에 차트를 추가하려면?`,
        `"${name}" 데이터를 요약해 주세요`,
        `대시보드 위젯 종류에는 어떤 것이 있나요?`,
      ]
    }
    if (isProcess) {
      return [
        `"${name}" 프로세스 단계를 설정하려면?`,
        `상태 전이 조건은 어떻게 설정하나요?`,
        `자동화 규칙을 추가하는 방법은?`,
      ]
    }
    if (isSettings) {
      return [
        `"${name}" 앱 설정에서 뭘 바꿀 수 있나요?`,
        `접근 권한을 설정하려면?`,
        `필드를 추가하거나 수정하는 방법은?`,
      ]
    }
    if (isView) {
      return [
        `"${name}"에 데이터를 추가하려면?`,
        `"${name}" 필드를 수정하는 방법은?`,
        `뷰를 추가하거나 전환하려면 어떻게 하나요?`,
      ]
    }
    return DEFAULT_HINTS
  }, [collection, location.pathname, appId])

  const scrollToBottom = useCallback(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [])

  useEffect(() => {
    scrollToBottom()
  }, [messages, scrollToBottom])

  useEffect(() => {
    if (open && inputRef.current) {
      inputRef.current.focus()
    }
  }, [open])

  function handleSend() {
    const text = input.trim()
    if (!text || chatMutation.isPending) return

    const userMsg: ChatMessage = { role: 'user', content: text }
    const newMessages = [...messages, userMsg]
    setMessages(newMessages)
    setInput('')

    chatMutation.mutate(
      { message: text, history: messages },
      {
        onSuccess: (data) => {
          setMessages((prev) => [
            ...prev,
            { role: 'assistant', content: data.reply },
          ])
        },
        onError: () => {
          setMessages((prev) => [
            ...prev,
            {
              role: 'assistant',
              content: 'AI 서버에 연결할 수 없습니다. 잠시 후 다시 시도해 주세요.',
            },
          ])
        },
      },
    )
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      handleSend()
    }
  }

  // Hide floating panel on the dedicated AI chat page.
  if (!aiAvailable || location.pathname === '/ai') return null

  return (
    <>
      {/* Floating trigger button */}
      {!open && (
        <button
          onClick={() => setOpen(true)}
          aria-label="AI 채팅 열기"
          className="fixed bottom-6 right-6 z-50 flex h-12 w-12 items-center justify-center rounded-full border border-stone-200 bg-white shadow-lg transition-transform hover:scale-105 active:scale-95"
        >
          <MessageCircle className="h-5 w-5 text-stone-700" />
        </button>
      )}

      {/* Chat panel */}
      {open && (
        <div className="fixed bottom-6 right-6 z-50 flex h-[520px] w-[380px] flex-col rounded-xl border border-stone-200 bg-white shadow-2xl">
          {/* Header */}
          <div className="flex items-center justify-between border-b px-4 py-3">
            <div className="flex items-center gap-2">
              <MessageCircle className="h-4 w-4 text-stone-500" />
              <span className="text-sm font-medium">Topworks 도우미</span>
            </div>
            <button
              onClick={() => setOpen(false)}
              className="rounded p-1 text-stone-400 hover:bg-stone-100 hover:text-stone-600"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          {/* Messages */}
          <div
            ref={scrollRef}
            className="flex-1 space-y-3 overflow-y-auto px-4 py-3"
          >
            {messages.length === 0 && (
              <div className="flex h-full flex-col items-center justify-center text-center">
                <MessageCircle className="mb-2 h-8 w-8 text-stone-300" />
                <p className="text-sm text-stone-400">
                  Topworks 사용에 대해 궁금한 점을 물어보세요.
                </p>
                <div className="mt-3 space-y-1.5">
                  {hints.map((hint) => (
                    <button
                      key={hint}
                      onClick={() => {
                        setInput(hint)
                        inputRef.current?.focus()
                      }}
                      className="block w-full rounded-md border px-3 py-1.5 text-left text-xs text-stone-500 transition-colors hover:bg-stone-50"
                    >
                      {hint}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {messages.map((msg, i) => (
              <div
                key={i}
                className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                <div
                  className={`max-w-[85%] rounded-lg px-3 py-2 text-sm ${
                    msg.role === 'user'
                      ? 'whitespace-pre-wrap bg-stone-900 text-white'
                      : 'bg-stone-100 text-stone-800'
                  }`}
                >
                  {msg.role === 'assistant' ? (
                    <div className="prose prose-sm prose-stone max-w-none [&>*:first-child]:mt-0 [&>*:last-child]:mb-0">
                      <Markdown>{msg.content}</Markdown>
                    </div>
                  ) : (
                    msg.content
                  )}
                </div>
              </div>
            ))}

            {chatMutation.isPending && (
              <div className="flex justify-start">
                <div className="flex items-center gap-1.5 rounded-lg bg-stone-100 px-3 py-2 text-sm text-stone-500">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  답변 생성 중...
                </div>
              </div>
            )}
          </div>

          {/* Input */}
          <div className="border-t px-3 py-2">
            <div className="flex items-end gap-2">
              <textarea
                ref={inputRef}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="질문을 입력하세요..."
                rows={1}
                className="max-h-24 min-h-[36px] flex-1 resize-none rounded-md border border-stone-200 px-3 py-2 text-sm outline-none placeholder:text-stone-400 focus:border-stone-400"
              />
              <Button
                size="sm"
                onClick={handleSend}
                disabled={!input.trim() || chatMutation.isPending}
                className="h-9 w-9 shrink-0 p-0"
              >
                <Send className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
