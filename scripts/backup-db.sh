#!/usr/bin/env bash
#
# backup-db.sh — PostgreSQL 백업 → Dropbox 업로드
#
# 필수 환경변수:
#   DATABASE_URL          PostgreSQL 연결 문자열
#   DROPBOX_REFRESH_TOKEN Dropbox OAuth2 리프레시 토큰
#   DROPBOX_APP_KEY       Dropbox 앱 키
#   DROPBOX_APP_SECRET    Dropbox 앱 시크릿
#
# 선택 환경변수:
#   DROPBOX_BACKUP_PATH   Dropbox 내 백업 폴더 (기본: /phaeton-backups)
#   BACKUP_RETAIN_DAYS    보관 일수 (기본: 30)
#
set -euo pipefail

# ── 설정 ──────────────────────────────────────────────
DATABASE_URL="${DATABASE_URL:?DATABASE_URL is required}"
DROPBOX_REFRESH_TOKEN="${DROPBOX_REFRESH_TOKEN:?DROPBOX_REFRESH_TOKEN is required}"
DROPBOX_APP_KEY="${DROPBOX_APP_KEY:?DROPBOX_APP_KEY is required}"
DROPBOX_APP_SECRET="${DROPBOX_APP_SECRET:?DROPBOX_APP_SECRET is required}"
DROPBOX_BACKUP_PATH="${DROPBOX_BACKUP_PATH:-/phaeton-backups}"
BACKUP_RETAIN_DAYS="${BACKUP_RETAIN_DAYS:-30}"

TIMESTAMP="$(date -u +%Y%m%d-%H%M%S)"
FILENAME="phaeton-${TIMESTAMP}.sql.gz"
TMPFILE="$(mktemp /tmp/phaeton-backup-XXXXXX.sql.gz)"

cleanup() { rm -f "$TMPFILE"; }
trap cleanup EXIT

log() { echo "[backup] $(date -u +%H:%M:%S) $*"; }

# ── 1. pg_dump ────────────────────────────────────────
log "Starting pg_dump..."
pg_dump "$DATABASE_URL" --no-owner --no-acl | gzip -9 > "$TMPFILE"
FILESIZE=$(stat -c%s "$TMPFILE" 2>/dev/null || stat -f%z "$TMPFILE")
log "Dump complete: ${FILENAME} ($(( FILESIZE / 1024 )) KB)"

# ── 2. Dropbox 액세스 토큰 발급 ───────────────────────
log "Requesting Dropbox access token..."
TOKEN_RESPONSE=$(curl -sS -X POST "https://api.dropboxapi.com/oauth2/token" \
  -d "grant_type=refresh_token" \
  -d "refresh_token=${DROPBOX_REFRESH_TOKEN}" \
  -d "client_id=${DROPBOX_APP_KEY}" \
  -d "client_secret=${DROPBOX_APP_SECRET}")

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | grep -o '"access_token"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$ACCESS_TOKEN" ]; then
  log "ERROR: Failed to get access token"
  echo "$TOKEN_RESPONSE" >&2
  exit 1
fi
log "Access token acquired"

# ── 3. Dropbox 업로드 ────────────────────────────────
DEST_PATH="${DROPBOX_BACKUP_PATH}/${FILENAME}"
log "Uploading to Dropbox: ${DEST_PATH}..."

if [ "$FILESIZE" -le 150000000 ]; then
  # 150MB 이하: 단일 업로드
  UPLOAD_RESPONSE=$(curl -sS -X POST "https://content.dropboxapi.com/2/files/upload" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Dropbox-API-Arg: {\"path\":\"${DEST_PATH}\",\"mode\":\"add\",\"autorename\":true}" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TMPFILE")

  if echo "$UPLOAD_RESPONSE" | grep -q '"id"'; then
    log "Upload successful"
  else
    log "ERROR: Upload failed"
    echo "$UPLOAD_RESPONSE" >&2
    exit 1
  fi
else
  # 150MB 초과: 세션 업로드
  CHUNK_SIZE=134217728  # 128MB
  OFFSET=0

  # 세션 시작
  SESSION_RESPONSE=$(head -c "$CHUNK_SIZE" "$TMPFILE" | \
    curl -sS -X POST "https://content.dropboxapi.com/2/files/upload_session/start" \
      -H "Authorization: Bearer ${ACCESS_TOKEN}" \
      -H "Dropbox-API-Arg: {\"close\":false}" \
      -H "Content-Type: application/octet-stream" \
      --data-binary @-)
  SESSION_ID=$(echo "$SESSION_RESPONSE" | grep -o '"session_id"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4)

  if [ -z "$SESSION_ID" ]; then
    log "ERROR: Failed to start upload session"
    echo "$SESSION_RESPONSE" >&2
    exit 1
  fi
  OFFSET=$CHUNK_SIZE

  # 중간 청크 전송
  while [ "$OFFSET" -lt "$((FILESIZE - CHUNK_SIZE))" ]; do
    dd if="$TMPFILE" bs=1 skip="$OFFSET" count="$CHUNK_SIZE" 2>/dev/null | \
      curl -sS -X POST "https://content.dropboxapi.com/2/files/upload_session/append_v2" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        -H "Dropbox-API-Arg: {\"cursor\":{\"session_id\":\"${SESSION_ID}\",\"offset\":${OFFSET}},\"close\":false}" \
        -H "Content-Type: application/octet-stream" \
        --data-binary @- > /dev/null
    OFFSET=$((OFFSET + CHUNK_SIZE))
  done

  # 마지막 청크 + 커밋
  dd if="$TMPFILE" bs=1 skip="$OFFSET" 2>/dev/null | \
    curl -sS -X POST "https://content.dropboxapi.com/2/files/upload_session/finish" \
      -H "Authorization: Bearer ${ACCESS_TOKEN}" \
      -H "Dropbox-API-Arg: {\"cursor\":{\"session_id\":\"${SESSION_ID}\",\"offset\":${OFFSET}},\"commit\":{\"path\":\"${DEST_PATH}\",\"mode\":\"add\",\"autorename\":true}}" \
      -H "Content-Type: application/octet-stream" \
      --data-binary @- > /dev/null

  log "Upload successful (chunked)"
fi

# ── 4. 오래된 백업 정리 ──────────────────────────────
log "Cleaning up backups older than ${BACKUP_RETAIN_DAYS} days..."
LIST_RESPONSE=$(curl -sS -X POST "https://api.dropboxapi.com/2/files/list_folder" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"path\":\"${DROPBOX_BACKUP_PATH}\",\"limit\":2000}")

CUTOFF_DATE=$(date -u -d "-${BACKUP_RETAIN_DAYS} days" +%Y%m%d 2>/dev/null || \
  date -u -v-${BACKUP_RETAIN_DAYS}d +%Y%m%d)

DELETED=0
for entry in $(echo "$LIST_RESPONSE" | grep -o '"name"[[:space:]]*:[[:space:]]*"phaeton-[0-9]*-[0-9]*.sql.gz"' | cut -d'"' -f4); do
  FILE_DATE=$(echo "$entry" | grep -o '[0-9]\{8\}')
  if [ -n "$FILE_DATE" ] && [ "$FILE_DATE" -lt "$CUTOFF_DATE" ]; then
    curl -sS -X POST "https://api.dropboxapi.com/2/files/delete_v2" \
      -H "Authorization: Bearer ${ACCESS_TOKEN}" \
      -H "Content-Type: application/json" \
      -d "{\"path\":\"${DROPBOX_BACKUP_PATH}/${entry}\"}" > /dev/null
    DELETED=$((DELETED + 1))
    log "Deleted old backup: ${entry}"
  fi
done
log "Cleanup done: ${DELETED} old backup(s) removed"

log "Backup complete: ${DEST_PATH}"
