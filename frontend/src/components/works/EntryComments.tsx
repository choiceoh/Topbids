import { MessageSquare } from 'lucide-react'
import { useState } from 'react'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import EmptyState from '@/components/common/EmptyState'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { useComments, useCreateComment, useDeleteComment } from '@/hooks/useComments'
import { useCurrentUser } from '@/hooks/useAuth'

interface Props {
  slug: string
  recordId: string
}

export default function EntryComments({ slug, recordId }: Props) {
  const [commentBody, setCommentBody] = useState('')
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null)
  const { data: currentUser } = useCurrentUser()
  const { data: commentsData, isLoading } = useComments(slug, recordId)
  const createComment = useCreateComment(slug, recordId)
  const deleteComment = useDeleteComment(slug, recordId)

  return (
    <div className="space-y-3">
      {isLoading ? (
        Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="space-y-2 rounded-md border p-3">
            <div className="flex items-center justify-between">
              <Skeleton className="h-4 w-20" />
              <Skeleton className="h-3 w-24" />
            </div>
            <Skeleton className="h-4 w-full" />
          </div>
        ))
      ) : commentsData?.data?.length ? (
        commentsData.data.map((c) => (
          <div key={c.id} className="rounded-md border p-3 text-sm">
            <div className="flex items-center justify-between">
              <span className="font-medium">{c.user_name}</span>
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">
                  {new Date(c.created_at).toLocaleString('ko')}
                </span>
                {(currentUser?.id === c.user_id || currentUser?.role === 'director') && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 px-2 text-xs"
                    onClick={() => setDeleteTargetId(c.id)}
                  >
                    삭제
                  </Button>
                )}
              </div>
            </div>
            <p className="mt-1 whitespace-pre-wrap">{c.body}</p>
          </div>
        ))
      ) : (
        <EmptyState
          compact
          icon={<MessageSquare className="h-8 w-8" />}
          title="아직 댓글이 없습니다"
          description="댓글을 남겨 팀원들과 소통하세요."
        />
      )}
      <div className="space-y-2">
        <Textarea
          placeholder="댓글을 입력하세요..."
          value={commentBody}
          onChange={(e) => setCommentBody(e.target.value)}
          rows={3}
        />
        <Button
          size="sm"
          disabled={!commentBody.trim() || createComment.isPending}
          onClick={() => {
            createComment.mutate(commentBody.trim(), {
              onSuccess: () => setCommentBody(''),
            })
          }}
        >
          댓글 작성
        </Button>
      </div>
      <ConfirmDialog
        open={deleteTargetId !== null}
        onOpenChange={(open) => { if (!open) setDeleteTargetId(null) }}
        title="댓글을 삭제하시겠습니까?"
        description="삭제된 댓글은 복구할 수 없습니다."
        variant="destructive"
        confirmLabel="삭제"
        loading={deleteComment.isPending}
        onConfirm={() => {
          if (deleteTargetId !== null) {
            deleteComment.mutate(deleteTargetId, {
              onSuccess: () => setDeleteTargetId(null),
            })
          }
        }}
      />
    </div>
  )
}
