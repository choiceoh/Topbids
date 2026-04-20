# 저장 피드백 — 시각적 확인 개선

## Context

현재 인라인 편집/CSV 가져오기 시 `toast.success`로 결과만 알려주고, **저장 진행 중** 상태가 없다. 사용자가 저장이 되었는지 불안해하는 문제를 해결한다.

## 변경 범위

### 1. 인라인 셀 편집 — 셀 레벨 저장 상태 표시
**파일**: `frontend/src/components/common/GridCell.tsx`

- 셀 저장 중일 때 짧은 시각 피드백 추가:
  - `saving` prop 추가 → 셀에 subtle spinner 또는 pulse 애니메이션
  - 저장 완료 시 잠시(~1초) 체크마크 또는 초록 배경 flash
- Toast는 제거하지 않되, 단건 인라인 편집 시 toast를 생략하고 셀 피드백으로 대체 (빈번한 토스트 피로 방지)

**파일**: `frontend/src/pages/AppViewPage.tsx` (307-318)
- `handleCellEdit`에서 저장 중인 셀 ID 상태 관리
- 성공 시 셀에 완료 상태 전달, 단건 toast 제거

### 2. 배치 편집 (붙여넣기) — toast.loading → toast.success 전환
**파일**: `frontend/src/pages/AppViewPage.tsx` (321-337)

- `handleBatchCellEdit`에서 `toast.loading` → 완료 후 `toast.success`로 교체
- Sonner의 `toast.loading()` + `toast.dismiss()` 패턴 활용

### 3. CSV 가져오기 — 로딩 상태 추가
**파일**: `frontend/src/pages/AppViewPage.tsx` (423-446)

- `importingCSV` 상태 추가
- `toast.loading('CSV 가져오는 중...')` 표시 → 완료 시 dismiss + success
- 가져오기 버튼에 disabled + spinner 표시

### 4. 엔트리 생성/수정 (폼) — 버튼 로딩 상태
**파일**: `frontend/src/pages/AppViewPage.tsx` (886)

- 이미 `submitting` prop이 전달되고 있음 → EntrySheet/EntryForm에서 이를 활용하는지 확인 & 강화

## 구현 상세

### GridCell 셀 피드백
```
GridCell props 추가: saving?: boolean, saved?: boolean
- saving=true → 오른쪽에 작은 Loader2 spinner (animate-spin, size-3)
- saved=true → 짧은 초록 체크 아이콘 (CheckIcon, size-3), 1초 후 fade out
- 두 상태 모두 아니면 기존 그대로
```

### AppViewPage 상태 관리
```
const [cellSaveState, setCellSaveState] = useState<Map<string, 'saving' | 'saved'>>()

handleCellEdit: 
  1. setCellSaveState에 `${rowId}:${columnId}` → 'saving' 설정
  2. onSuccess → 'saved' 설정, 1초 후 제거. toast 제거
  3. onError → 제거 + toast.error 유지
```

### DataTable에 cellSaveState 전달
- `DataTable` Props에 `cellSaveState?: Map<string, 'saving' | 'saved'>` 추가
- DataTable 내부 GridCell 렌더링 시 `saving`/`saved` prop 계산:
  ```
  const cellKey = `${rowId}:${colId}`
  saving={cellSaveState?.get(cellKey) === 'saving'}
  saved={cellSaveState?.get(cellKey) === 'saved'}
  ```
- `AppViewPage.tsx`에서 DataTable에 `cellSaveState` prop 전달

## 검증 방법
1. `make dev-ui` + `make dev-api` 실행
2. 앱 뷰에서 셀 더블클릭 → 값 변경 → blur → spinner 표시 후 체크 확인
3. 여러 셀 붙여넣기 → loading toast → success toast 전환 확인
4. CSV 파일 가져오기 → 로딩 표시 → 완료 메시지 확인
