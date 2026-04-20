# 07. 린트 및 코드 스타일 규칙

Phaeton 프로젝트 코드 포매팅, 네이밍, 린트 규칙 정의.
설정 파일: `eslint.config.mjs`, `.prettierrc`

---

## 1. 포매팅

Prettier로 자동 적용. `.prettierrc` 설정 기준.

| 규칙             | 값                                 |
| ---------------- | ---------------------------------- |
| 세미콜론         | 없음 (`"semi": false`)             |
| 따옴표           | 작은따옴표 (`"singleQuote": true`) |
| trailing comma   | 항상 (`"trailingComma": "all"`)    |
| 들여쓰기         | 2칸 스페이스 (`"tabWidth": 2`)     |
| 줄 폭            | 100자 (`"printWidth": 100`)        |
| 화살표 함수 괄호 | 항상 (`"arrowParens": "always"`)   |
| 줄바꿈           | LF (`"endOfLine": "lf"`)           |

플러그인: `prettier-plugin-tailwindcss` (Tailwind 클래스 자동 정렬)

---

## 2. 네이밍

| 대상            | 규칙       | 예시                                     |
| --------------- | ---------- | ---------------------------------------- |
| 컴포넌트 파일   | PascalCase | `ProjectCard.tsx`                        |
| 유틸/훅 파일    | camelCase  | `copyMilestones.ts`, `useProjectData.ts` |
| Collection slug | kebab-case | `project-milestones`                     |
| API 라우트      | kebab-case | `/api/phaeton/staff-load`                |

---

## 3. import 순서

`eslint-plugin-import-x`로 강제. 순서를 어기면 에러 발생.

```typescript
// 1. Node 내장 모듈
import path from 'node:path'

// 2. 외부 패키지
import { Card } from '@tremor/react'

// 3. @/ 내부 경로
import { getProjectStats } from '@/lib/queries'

// 4. 상대 경로
import { StatusBadge } from './StatusBadge'
```

- 그룹 간 **빈 줄 필수**
- 그룹 내 알파벳 오름차순 정렬 (대소문자 무시)
- `@/**` 패턴은 internal 그룹으로 분류
- `@payload-config`도 internal 그룹

---

## 4. 제한 사항

### TypeScript

| 규칙                 | 설정          | 비고                                    |
| -------------------- | ------------- | --------------------------------------- |
| `no-explicit-any`    | `warn` (기본) | Collection/Hook 파일에서만 허용 (`off`) |
| `no-unused-vars`     | `error`       | `_` 접두사로 예외 처리 가능             |
| `no-require-imports` | `error`       | ESM `import`만 사용                     |

### JavaScript

| 규칙               | 설정    | 비고                                   |
| ------------------ | ------- | -------------------------------------- |
| `no-console`       | `warn`  | `console.warn`, `console.error`만 허용 |
| `eqeqeq`           | `error` | `===` 강제. `==` 사용 금지             |
| `no-var`           | `error` | `let` 또는 `const` 사용                |
| `prefer-const`     | `error` | 재할당 없으면 `const`                  |
| `prefer-template`  | `error` | 문자열 연결 대신 템플릿 리터럴         |
| `arrow-body-style` | `warn`  | 가능하면 간결한 화살표 함수 본문       |

### any 허용 범위

`@typescript-eslint/no-explicit-any`가 `off`인 파일:

- `src/collections/**/*.ts`
- `src/hooks/**/*.ts`

위 경로 외에서 `any`를 사용하면 린트 경고 발생.

---

## 5. 도구 설정

### ESLint (`eslint.config.mjs`)

- `typescript-eslint` flat config 방식
- `eslint-config-prettier`로 Prettier와 충돌 방지
- `eslint-plugin-import-x`로 import 순서 강제

린트 무시 대상:

- `payload-types.ts` (자동 생성)
- `migrations/**`
- `node_modules/**`, `.next/**`, `dist/**`
- `src/app/(payload)/**` (Payload Admin 영역)

### Prettier (`.prettierrc`)

위 포매팅 섹션 참조. `prettier-plugin-tailwindcss` 포함.

### 실행 명령어

```bash
# 린트 검사
npx eslint .

# 린트 자동 수정
npx eslint . --fix

# 포매팅 검사
npx prettier --check .

# 포매팅 자동 적용
npx prettier --write .
```
