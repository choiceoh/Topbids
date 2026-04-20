import { useCallback, useEffect, useRef, useState } from 'react'
import { ImagePlus, Loader2, MessageCircle, RotateCcw, Send, X } from 'lucide-react'
import Markdown from 'react-markdown'

import { Button } from '@/components/ui/button'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useAIChat, type ChatMessage } from '@/hooks/useAIChat'

const HINTS = [
  '앱을 만들려면 어떻게 하나요?',
  '현재 어떤 앱들이 있나요?',
  '항목 유형에는 어떤 것들이 있나요?',
  '데이터를 필터링하려면 어떻게 하나요?',
  '자동화 규칙을 설정하는 방법은?',
  '뷰를 추가하거나 전환하려면?',
]

export default function AIChatPage() {
  const aiAvailable = useAIAvailable()
  const [input, setInput] = useState('')
  const [images, setImages] = useState<string[]>([])
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const chatMutation = useAIChat()
  const pendingRef = useRef(0)

  const scrollToBottom = useCallback(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [])

  useEffect(() => {
    scrollToBottom()
  }, [messages, scrollToBottom])

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  function handleImageSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const files = e.target.files
    if (!files) return
    for (const file of Array.from(files)) {
      if (!file.type.startsWith('image/')) continue
      const reader = new FileReader()
      reader.onload = () => {
        setImages((prev) => [...prev, reader.result as string])
      }
      reader.readAsDataURL(file)
    }
    e.target.value = ''
  }

  function handleSend(text?: string) {
    const msg = (text ?? input).trim()
    if (!msg && images.length === 0) return
    if (chatMutation.isPending) return

    const currentImages = images.length > 0 ? [...images] : undefined
    const userMsg: ChatMessage = { role: 'user', content: msg || '이 이미지를 분석해 주세요.', images: currentImages }
    const newMessages = [...messages, userMsg]
    setMessages(newMessages)
    setInput('')
    setImages([])
    if (inputRef.current) inputRef.current.style.height = ''

    const seq = ++pendingRef.current
    chatMutation.mutate(
      { message: userMsg.content, history: messages, images: currentImages },
      {
        onSuccess: (data) => {
          if (seq !== pendingRef.current) return
          setMessages((prev) => [
            ...prev,
            { role: 'assistant', content: data.reply },
          ])
        },
        onError: () => {
          if (seq !== pendingRef.current) return
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

  function autoResize() {
    const el = inputRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 128)}px`
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      handleSend()
    }
  }

  if (!aiAvailable) {
    return (
      <div className="flex h-[60vh] flex-col items-center justify-center text-stone-400">
        <MessageCircle className="mb-3 h-10 w-10" />
        <p className="text-sm">AI 서버에 연결할 수 없습니다.</p>
        <p className="text-xs">잠시 후 다시 시도해 주세요.</p>
      </div>
    )
  }

  return (
    <div className="mx-auto flex h-[calc(100vh-10rem)] max-w-3xl flex-col">
      {/* Header */}
      {messages.length > 0 && (
        <div className="flex items-center justify-end py-2">
          <button
            onClick={() => { pendingRef.current++; setMessages([]); setInput(''); setImages([]) }}
            className="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs text-stone-400 transition-colors hover:bg-stone-100 hover:text-stone-600"
          >
            <RotateCcw className="h-3.5 w-3.5" />
            새 대화
          </button>
        </div>
      )}

      {/* Messages area */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto py-4">
        {messages.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center text-center">
            <MessageCircle className="mb-3 h-10 w-10 text-stone-300" />
            <h2 className="text-lg font-medium text-stone-700">Topworks AI</h2>
            <p className="mt-1 text-sm text-stone-400">
              Topworks 사용에 대해 궁금한 점을 물어보세요.
            </p>
            <div className="mt-6 grid grid-cols-2 gap-2">
              {HINTS.map((hint) => (
                <button
                  key={hint}
                  onClick={() => handleSend(hint)}
                  className="rounded-lg border border-stone-200 px-4 py-2.5 text-left text-sm text-stone-600 transition-colors hover:bg-stone-50"
                >
                  {hint}
                </button>
              ))}
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            {messages.map((msg, i) => (
              <div
                key={i}
                className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                <div
                  className={`max-w-[80%] rounded-lg px-4 py-3 text-sm ${
                    msg.role === 'user'
                      ? 'whitespace-pre-wrap bg-stone-900 text-white'
                      : 'bg-stone-100 text-stone-800'
                  }`}
                >
                  {msg.role === 'user' && msg.images && msg.images.length > 0 && (
                    <div className="mb-2 flex flex-wrap gap-1.5">
                      {msg.images.map((src, j) => (
                        <img key={j} src={src} alt="" className="max-h-40 rounded border border-white/20 object-cover" />
                      ))}
                    </div>
                  )}
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
                <div className="flex items-center gap-2 rounded-lg bg-stone-100 px-4 py-3 text-sm text-stone-500">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  답변 생성 중...
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Input area */}
      <div className="border-t border-stone-200 pt-4 pb-2">
        {images.length > 0 && (
          <div className="mb-2 flex flex-wrap gap-2">
            {images.map((src, i) => (
              <div key={i} className="group relative">
                <img src={src} alt="" className="h-16 w-16 rounded-md border border-stone-200 object-cover" />
                <button
                  onClick={() => setImages((prev) => prev.filter((_, j) => j !== i))}
                  className="absolute -top-1.5 -right-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-stone-800 text-white opacity-0 transition-opacity group-hover:opacity-100"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        )}
        <div className="flex items-end gap-3">
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            multiple
            onChange={handleImageSelect}
            className="hidden"
          />
          <button
            onClick={() => fileInputRef.current?.click()}
            className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-stone-200 text-stone-400 transition-colors hover:bg-stone-50 hover:text-stone-600"
          >
            <ImagePlus className="h-4.5 w-4.5" />
          </button>
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => { setInput(e.target.value); autoResize() }}
            onKeyDown={handleKeyDown}
            placeholder="질문을 입력하세요..."
            rows={1}
            className="max-h-32 min-h-[44px] flex-1 resize-none rounded-lg border border-stone-200 px-4 py-3 text-sm outline-none placeholder:text-stone-400 focus:border-stone-400"
          />
          <Button
            onClick={() => handleSend()}
            disabled={(!input.trim() && images.length === 0) || chatMutation.isPending}
            className="h-11 w-11 shrink-0 p-0"
          >
            <Send className="h-4 w-4" />
          </Button>
        </div>
        <p className="mt-2 text-center text-xs text-stone-400">
          로컬 AI가 응답합니다. 답변이 정확하지 않을 수 있습니다.
        </p>
      </div>
    </div>
  )
}
