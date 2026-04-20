import type { FieldType } from '@/lib/types'

export interface FieldHint {
  description: string
  example: string
}

export const FIELD_HINTS: Partial<Record<FieldType, FieldHint>> = {
  text: { description: '짧은 텍스트 입력', example: '예: 고객명, 프로젝트명' },
  textarea: { description: '여러 줄 텍스트 입력', example: '예: 상세 설명, 비고' },
  number: { description: '숫자 값 입력', example: '예: 100, 3.14' },
  integer: { description: '정수 값 입력', example: '예: 1, 50, 300' },
  boolean: { description: '예/아니오 체크', example: '예: 완료 여부, 승인 여부' },
  date: { description: '날짜 선택', example: '예: 마감일, 시작일' },
  datetime: { description: '날짜 + 시간 선택', example: '예: 회의 일시' },
  time: { description: '시간 선택', example: '예: 출퇴근 시각' },
  select: { description: '옵션 중 하나 선택', example: '예: 상태, 카테고리' },
  multiselect: { description: '여러 옵션 선택', example: '예: 태그, 담당 부서' },
  relation: { description: '다른 앱과 연결', example: '예: 소속 프로젝트' },
  user: { description: '사용자 선택', example: '예: 담당자, 승인자' },
  file: { description: '파일 첨부', example: '예: 계약서, 이미지' },
  json: { description: 'JSON 구조화 데이터', example: '예: 설정값, 메타데이터' },
  autonumber: { description: '자동 증가 번호', example: '예: 접수번호, 문서번호' },
  formula: { description: '다른 항목 참조 수식', example: '예: {단가} * {수량}' },
  lookup: { description: '연결된 레코드 값 조회', example: '예: 프로젝트의 상태' },
  rollup: { description: '연결 레코드 집계', example: '예: 하위 항목 합계' },
  label: { description: '안내 텍스트 표시', example: '입력란 없이 설명만 표시' },
  line: { description: '구분선 삽입', example: '영역을 시각적으로 구분' },
  spacer: { description: '여백 추가', example: '항목 간 간격 조정' },
  table: { description: '인라인 테이블', example: '예: 품목 목록, 일정표' },
  spreadsheet: { description: '엑셀 스프레드시트', example: '예: 견적서, 정산표' },
}
