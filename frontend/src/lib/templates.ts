import type { CreateCollectionReq } from './types'

export interface Template {
  id: string
  category: TemplateCategory
  label: string
  description: string
  collection: CreateCollectionReq
}

export type TemplateCategory = '총무' | '인사' | '영업' | '재무'

export const TEMPLATE_CATEGORIES: TemplateCategory[] = ['총무', '인사', '영업', '재무']

export const TEMPLATES: Template[] = [
  // --- 총무 ---
  {
    id: 'asset_management',
    category: '총무',
    label: '자산 관리',
    description: '사내 비품, IT 장비 등 자산을 등록하고 현황을 추적합니다.',
    collection: {
      slug: 'asset_management',
      label: '자산 관리',
      description: '사내 비품 및 IT 장비 관리',
      fields: [
        { slug: 'name', label: '자산명', field_type: 'text', is_required: true, width: 3 },
        { slug: 'category', label: '분류', field_type: 'select', width: 3, options: { choices: ['IT장비', '가구', '차량', '사무용품', '기타'] } },
        { slug: 'serial_number', label: '시리얼번호', field_type: 'text', width: 3 },
        { slug: 'status', label: '상태', field_type: 'select', width: 3, options: { choices: ['사용중', '보관중', '수리중', '폐기'] } },
        { slug: 'purchase_date', label: '구매일', field_type: 'date', width: 3 },
        { slug: 'purchase_price', label: '구매가', field_type: 'number', width: 3, options: { display_type: 'currency' } },
        { slug: 'location', label: '위치', field_type: 'text', width: 3 },
        { slug: 'assignee', label: '사용자', field_type: 'text', width: 3 },
        { slug: 'photo', label: '사진', field_type: 'file', width: 6 },
        { slug: 'note', label: '비고', field_type: 'textarea', width: 6 },
      ],
    },
  },
  {
    id: 'meeting_room',
    category: '총무',
    label: '회의실 예약',
    description: '회의실 예약 현황을 관리합니다.',
    collection: {
      slug: 'meeting_room',
      label: '회의실 예약',
      description: '회의실 예약 관리',
      fields: [
        { slug: 'title', label: '회의명', field_type: 'text', is_required: true, width: 6 },
        { slug: 'room', label: '회의실', field_type: 'select', width: 3, options: { choices: ['대회의실', '소회의실A', '소회의실B', '화상회의실'] } },
        { slug: 'date', label: '날짜', field_type: 'date', is_required: true, width: 2 },
        { slug: 'start_time', label: '시작 시간', field_type: 'time', width: 2 },
        { slug: 'end_time', label: '종료 시간', field_type: 'time', width: 2 },
        { slug: 'organizer', label: '주최자', field_type: 'text', width: 3 },
        { slug: 'attendees', label: '참석자', field_type: 'textarea', width: 3 },
        { slug: 'note', label: '메모', field_type: 'textarea', width: 6 },
      ],
    },
  },
  {
    id: 'supply_request',
    category: '총무',
    label: '비품 요청',
    description: '사무용품 및 비품 구매 요청을 접수하고 처리합니다.',
    collection: {
      slug: 'supply_request',
      label: '비품 요청',
      description: '비품 구매 요청 관리',
      fields: [
        { slug: 'item_name', label: '품목', field_type: 'text', is_required: true, width: 3 },
        { slug: 'quantity', label: '수량', field_type: 'integer', is_required: true, width: 2 },
        { slug: 'purpose', label: '용도', field_type: 'textarea', width: 6 },
        { slug: 'urgency', label: '긴급도', field_type: 'select', width: 2, options: { choices: ['보통', '긴급', '매우긴급'] } },
        { slug: 'status', label: '처리 상태', field_type: 'select', width: 3, options: { choices: ['요청', '검토중', '승인', '구매완료', '반려'] } },
        { slug: 'requester', label: '요청자', field_type: 'text', width: 3 },
        { slug: 'request_date', label: '요청일', field_type: 'date', width: 3 },
      ],
    },
  },

  // --- 인사 ---
  {
    id: 'employee_directory',
    category: '인사',
    label: '직원 명부',
    description: '직원 기본 정보와 연락처를 관리합니다.',
    collection: {
      slug: 'employee_directory',
      label: '직원 명부',
      description: '직원 정보 관리',
      fields: [
        { slug: 'name', label: '이름', field_type: 'text', is_required: true, width: 3 },
        { slug: 'department', label: '부서', field_type: 'select', width: 3, options: { choices: ['경영지원', '개발', '영업', '마케팅', '디자인', '인사'] } },
        { slug: 'position', label: '직급', field_type: 'select', width: 2, options: { choices: ['사원', '주임', '대리', '과장', '차장', '부장', '이사'] } },
        { slug: 'email', label: '이메일', field_type: 'text', width: 3, options: { display_type: 'email' } },
        { slug: 'phone', label: '연락처', field_type: 'text', width: 3, options: { display_type: 'phone' } },
        { slug: 'join_date', label: '입사일', field_type: 'date', width: 3 },
        { slug: 'status', label: '재직 상태', field_type: 'select', width: 2, options: { choices: ['재직', '휴직', '퇴직'] } },
        { slug: 'photo', label: '사진', field_type: 'file', width: 6 },
      ],
    },
  },
  {
    id: 'leave_request',
    category: '인사',
    label: '휴가 신청',
    description: '연차, 반차, 특별 휴가 등을 신청하고 승인합니다.',
    collection: {
      slug: 'leave_request',
      label: '휴가 신청',
      description: '휴가 신청 및 승인 관리',
      fields: [
        { slug: 'requester', label: '신청자', field_type: 'text', is_required: true, width: 3 },
        { slug: 'leave_type', label: '휴가 유형', field_type: 'select', is_required: true, width: 3, options: { choices: ['연차', '반차(오전)', '반차(오후)', '병가', '경조사', '공가', '기타'] } },
        { slug: 'start_date', label: '시작일', field_type: 'date', is_required: true, width: 3 },
        { slug: 'end_date', label: '종료일', field_type: 'date', is_required: true, width: 3 },
        { slug: 'days', label: '일수', field_type: 'number', width: 2 },
        { slug: 'reason', label: '사유', field_type: 'textarea', width: 6 },
        { slug: 'status', label: '승인 상태', field_type: 'select', width: 2, options: { choices: ['신청', '승인', '반려', '취소'] } },
        { slug: 'approver', label: '승인자', field_type: 'text', width: 3 },
      ],
    },
  },
  {
    id: 'recruitment',
    category: '인사',
    label: '채용 관리',
    description: '지원자 정보와 채용 프로세스를 관리합니다.',
    collection: {
      slug: 'recruitment',
      label: '채용 관리',
      description: '채용 프로세스 관리',
      fields: [
        { slug: 'applicant_name', label: '지원자', field_type: 'text', is_required: true, width: 3 },
        { slug: 'position', label: '지원 직무', field_type: 'text', is_required: true, width: 3 },
        { slug: 'email', label: '이메일', field_type: 'text', width: 3, options: { display_type: 'email' } },
        { slug: 'phone', label: '연락처', field_type: 'text', width: 3, options: { display_type: 'phone' } },
        { slug: 'apply_date', label: '지원일', field_type: 'date', width: 3 },
        { slug: 'stage', label: '전형 단계', field_type: 'select', width: 3, options: { choices: ['서류심사', '1차면접', '2차면접', '최종합격', '불합격'] } },
        { slug: 'resume', label: '이력서', field_type: 'file', width: 6 },
        { slug: 'note', label: '메모', field_type: 'textarea', width: 6 },
      ],
    },
  },

  // --- 영업 ---
  {
    id: 'customer_list',
    category: '영업',
    label: '고객 관리',
    description: '고객사 정보와 담당자를 관리합니다.',
    collection: {
      slug: 'customer_list',
      label: '고객 관리',
      description: '고객사 및 담당자 관리',
      fields: [
        { slug: 'company', label: '회사명', field_type: 'text', is_required: true, width: 3 },
        { slug: 'contact_name', label: '담당자', field_type: 'text', width: 3 },
        { slug: 'email', label: '이메일', field_type: 'text', width: 3, options: { display_type: 'email' } },
        { slug: 'phone', label: '연락처', field_type: 'text', width: 3, options: { display_type: 'phone' } },
        { slug: 'industry', label: '업종', field_type: 'select', width: 3, options: { choices: ['IT', '제조', '유통', '금융', '서비스', '공공', '기타'] } },
        { slug: 'grade', label: '등급', field_type: 'select', width: 2, options: { choices: ['VIP', 'A', 'B', 'C'] } },
        { slug: 'address', label: '주소', field_type: 'text', width: 6 },
        { slug: 'note', label: '비고', field_type: 'textarea', width: 6 },
      ],
    },
  },
  {
    id: 'sales_pipeline',
    category: '영업',
    label: '영업 파이프라인',
    description: '영업 기회를 단계별로 추적합니다.',
    collection: {
      slug: 'sales_pipeline',
      label: '영업 파이프라인',
      description: '영업 기회 추적',
      fields: [
        { slug: 'deal_name', label: '건명', field_type: 'text', is_required: true, width: 6 },
        { slug: 'customer', label: '고객사', field_type: 'text', is_required: true, width: 3 },
        { slug: 'amount', label: '예상 금액', field_type: 'number', width: 3, options: { display_type: 'currency' } },
        { slug: 'stage', label: '단계', field_type: 'select', is_required: true, width: 3, options: { choices: ['리드', '제안', '협상', '계약', '수주', '실주'] } },
        { slug: 'probability', label: '성공률', field_type: 'integer', width: 2, options: { display_type: 'progress' } },
        { slug: 'owner', label: '담당자', field_type: 'text', width: 3 },
        { slug: 'expected_close', label: '예상 마감일', field_type: 'date', width: 3 },
        { slug: 'note', label: '비고', field_type: 'textarea', width: 6 },
      ],
    },
  },
  {
    id: 'quotation',
    category: '영업',
    label: '견적 관리',
    description: '견적서를 생성하고 이력을 관리합니다.',
    collection: {
      slug: 'quotation',
      label: '견적 관리',
      description: '견적서 관리',
      fields: [
        { slug: 'quote_no', label: '견적번호', field_type: 'text', is_required: true, is_unique: true, width: 3 },
        { slug: 'customer', label: '고객사', field_type: 'text', is_required: true, width: 3 },
        { slug: 'title', label: '건명', field_type: 'text', is_required: true, width: 6 },
        { slug: 'amount', label: '견적 금액', field_type: 'number', width: 3, options: { display_type: 'currency' } },
        { slug: 'issue_date', label: '발행일', field_type: 'date', width: 3 },
        { slug: 'valid_until', label: '유효기간', field_type: 'date', width: 3 },
        { slug: 'status', label: '상태', field_type: 'select', width: 3, options: { choices: ['작성중', '발송', '승인', '만료', '취소'] } },
        { slug: 'attachment', label: '첨부파일', field_type: 'file', width: 6 },
      ],
    },
  },

  // --- 재무 ---
  {
    id: 'expense_report',
    category: '재무',
    label: '경비 정산',
    description: '법인카드, 개인 경비 등의 지출을 정산합니다.',
    collection: {
      slug: 'expense_report',
      label: '경비 정산',
      description: '경비 정산 관리',
      fields: [
        { slug: 'title', label: '항목', field_type: 'text', is_required: true, width: 6 },
        { slug: 'category', label: '분류', field_type: 'select', width: 3, options: { choices: ['교통비', '식비', '숙박비', '접대비', '소모품', '기타'] } },
        { slug: 'amount', label: '금액', field_type: 'number', is_required: true, width: 3, options: { display_type: 'currency' } },
        { slug: 'expense_date', label: '지출일', field_type: 'date', is_required: true, width: 3 },
        { slug: 'payment_method', label: '결제 수단', field_type: 'select', width: 3, options: { choices: ['법인카드', '개인카드', '현금'] } },
        { slug: 'requester', label: '신청자', field_type: 'text', width: 3 },
        { slug: 'status', label: '처리 상태', field_type: 'select', width: 3, options: { choices: ['신청', '검토중', '승인', '지급완료', '반려'] } },
        { slug: 'receipt', label: '영수증', field_type: 'file', width: 6 },
        { slug: 'note', label: '비고', field_type: 'textarea', width: 6 },
      ],
    },
  },
  {
    id: 'budget_tracking',
    category: '재무',
    label: '예산 관리',
    description: '부서별 예산 배정과 집행 현황을 추적합니다.',
    collection: {
      slug: 'budget_tracking',
      label: '예산 관리',
      description: '예산 배정 및 집행 추적',
      fields: [
        { slug: 'department', label: '부서', field_type: 'select', is_required: true, width: 3, options: { choices: ['경영지원', '개발', '영업', '마케팅', '디자인', '인사'] } },
        { slug: 'category', label: '항목', field_type: 'text', is_required: true, width: 3 },
        { slug: 'budget', label: '배정 예산', field_type: 'number', is_required: true, width: 3, options: { display_type: 'currency' } },
        { slug: 'spent', label: '집행액', field_type: 'number', width: 3, options: { display_type: 'currency' } },
        { slug: 'remaining', label: '잔여액', field_type: 'number', width: 3, options: { display_type: 'currency' } },
        { slug: 'execution_rate', label: '집행률', field_type: 'integer', width: 2, options: { display_type: 'progress' } },
        { slug: 'period', label: '기간', field_type: 'select', width: 2, options: { choices: ['1분기', '2분기', '3분기', '4분기', '연간'] } },
        { slug: 'note', label: '비고', field_type: 'textarea', width: 6 },
      ],
    },
  },
  {
    id: 'invoice',
    category: '재무',
    label: '청구서 관리',
    description: '발행/수취 청구서를 추적하고 입금 상태를 관리합니다.',
    collection: {
      slug: 'invoice',
      label: '청구서 관리',
      description: '청구서 발행 및 수금 관리',
      fields: [
        { slug: 'invoice_no', label: '청구번호', field_type: 'text', is_required: true, is_unique: true, width: 3 },
        { slug: 'customer', label: '거래처', field_type: 'text', is_required: true, width: 3 },
        { slug: 'amount', label: '청구 금액', field_type: 'number', is_required: true, width: 3, options: { display_type: 'currency' } },
        { slug: 'issue_date', label: '발행일', field_type: 'date', is_required: true, width: 3 },
        { slug: 'due_date', label: '입금 기한', field_type: 'date', width: 3 },
        { slug: 'status', label: '상태', field_type: 'select', width: 3, options: { choices: ['발행', '발송', '입금완료', '연체', '취소'] } },
        { slug: 'attachment', label: '첨부파일', field_type: 'file', width: 6 },
      ],
    },
  },
]
