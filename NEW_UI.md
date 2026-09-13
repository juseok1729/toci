# toci 기획서: 컴파트먼트를 전역 컨텍스트로 분리

> 이 문서는 toci(Go, Bubble Tea/lipgloss 기반 OCI TUI)에 컴파트먼트 컨텍스트 스위칭 기능을 구현하기 위한 기획서이자 구현 프롬프트다. 아래 요구사항과 UI 스펙을 기준으로 설계·구현하고, 판단이 필요한 지점은 "구현 메모"의 방향을 따른다. UI 스펙은 **현재 toci 화면의 시각 언어(헤더 4줄, 타이틀 바 있는 테이블, 중앙 오버레이 피커, 푸터 힌트)를 그대로 재사용**하는 것을 전제로 한다.

---

## 1. 배경

### 사용자 피드백 (원문)
> 컴파트먼트에 모든 리소스가 속해있지만 컴파트먼트가 최상위 필터기 때문에 리소스 스위칭할 때 무조건 컴파트먼트 상속 리소스만 나온다. 근데 수시로 보고 있는 리소스에서 컴파트먼트만 교체할 수 있으면 좋겠다.

### 현재 UI 분석
현재 화면 구조:
```
Profile:     DEFAULT                                   TOCI!
Region:      ap-seoul-1                                OCI TUI
Resource:    Exadata VM Clusters (Exascale)
Compartment: ywdcloud/juseok.oh/hub-and-spoke › vcn-db
┌──────── Exadata VM Clusters (Exascale) [3] ────────┐
│ NAME  STATE  SHAPE  ...                            │
│ ExascaleRAC  Available ...                         │
│  ├─ rac-exascale2  Stopped                         │
└────────────────────────────────────────────────────┘
3 items · space: shortcuts · show nodes: on
```
- 리소스 전환은 중앙 오버레이 피커(`Resources`, fuzzy 입력, 우측 `16/16` 카운트)로 이미 잘 되어 있다.
- **"Compartments"가 리소스 타입 중 하나로 등록되어 있다.** 컴파트먼트를 바꾸려면 리소스 피커 → Compartments → 목록 탐색 → 진입의 4단계를 거치고, 그동안 보고 있던 리소스 뷰는 사라진다. 이것이 피드백의 직접 원인이다.
- 헤더에 `Compartment:` 라인과 경로가 이미 있고, `› vcn-db` 처럼 컴파트먼트 뒤에 하위 스코프가 다른 색으로 붙는다. 컨텍스트를 표시할 자리는 이미 마련되어 있는 셈이다.
- 테이블은 타이틀 바에 `[N]` 카운트를 표시하고, `show nodes` 토글로 부모–자식 트리 행(`├─`, `└─`)을 지원한다.

### 문제 정의
네비게이션 구조가 `compartment → resource type → list` 로, 컴파트먼트가 **네비게이션 단계**로 취급된다. 사용자 요구의 본질은 **"컴파트먼트는 네비게이션 단계가 아니라 필터다"** 이다. 필터는 어느 화면에서든 바꿀 수 있어야 한다.

---

## 2. 목표

- 컴파트먼트를 k9s의 namespace처럼 **뷰와 독립적인 전역 컨텍스트**로 승격한다.
- 축을 `(compartment, region) × resource type` 으로 분리해, 어느 축을 바꿔도 다른 축은 유지된다.
- 어떤 리소스 뷰에서든 화면을 떠나지 않고 컴파트먼트를 교체할 수 있다.
- OCI 컴파트먼트 계층 특성을 반영해 "현재 + 하위 전체" 조회를 지원한다.
- 기존 시각 언어를 유지한다. 새 위젯 스타일을 만들지 않고 헤더 라인·오버레이 피커·타이틀 바를 재사용한다.

### 비목표
- 컴파트먼트 생성/수정/삭제 등 관리 기능
- 리전 스위칭 UX 변경 (기존 유지, 같은 컨텍스트 헤더에 표시)
- Compartments 리소스 타입 제거 (남기되 역할을 바꾼다 — F6)

---

## 3. 설계 원칙

1. **컨텍스트는 항상 보인다** — 헤더의 `Region:` / `Resource:` / `Compartment:` 라인은 상시 표시. 컴파트먼트 경로와 하위 스코프(`› vcn-db`)는 색으로 구분을 유지한다.
2. **뷰를 떠나지 않는다** — 컨텍스트 변경은 오버레이 피커 또는 단축키로 처리하고, 완료 후 같은 리소스 타입 리스트를 재조회한다.
3. **부분 결과 우선** — fan-out 조회는 도착 순서대로 스트리밍 표시. 전체 완료를 기다리게 하지 않는다.
4. **모드는 두 곳에서 알린다** — 서브트리 ON 상태는 헤더 `Compartment:` 라인과 푸터 힌트 양쪽에 같은 색으로 표시.
5. **핫키는 보여야 쓴다** — 최근 컴파트먼트 숫자 키는 푸터가 아니라 헤더에 노출한다.

---

## 4. 기능 요구사항

### F1. 뷰 내 컴파트먼트 스위칭 (필수)
- 어떤 리소스 리스트 화면에서든 `c` (또는 `:cmp`) 로 컴파트먼트 피커 오버레이를 연다.
- 선택 시 현재 리소스 타입을 유지한 채 리스트만 재조회한다.
- 커서 위치·필터 문자열은 가능하면 유지(동일 리소스 ID가 있으면 커서 복원).
- 컴파트먼트 전환 시 하위 스코프(`› vcn-db` 등 VCN/서브넷 필터)는 초기화한다.

### F2. 최근 컴파트먼트 핫키 (필수)
- 헤더에 `Recent:` 라인을 추가하고 `1`~`9` 를 MRU 순으로 바인딩. 피커 없이 즉시 전환.
- 서브트리 포함 상태로 저장된 항목은 이름 뒤에 `⊕` 를 붙인다 (`1 juseok.oh⊕`).
- `0` 은 "전체 테넌시"(F4) 예약. 헤더 우측 끝에 `0 all` 로 dim 표시.
- MRU는 설정 파일에 영속화. 피커에서 `p` 로 pin 하면 MRU에서 밀리지 않는다.

### F3. 서브트리 포함 모드 (권장 — F1과 함께 설계)
- `C` 로 토글. ON 이면 현재 컴파트먼트 + 하위 컴파트먼트 전체를 fan-out 조회.
- 헤더 `Compartment:` 라인 뒤에 `⊕ +N sub` (accent 색), 푸터에 `C: subtree on` (같은 색).
- ON 상태에서만 테이블 맨 앞에 `COMPARTMENT` 컬럼이 추가된다. 값은 **현재 선택 기준 상대 경로**(`hub-and-spoke`, `db/backup`). `show nodes` 트리의 자식 행에는 비워둔다.
- 테이블 타이틀 바 우측에 `↻ 3/4 loaded` 로 부분 로딩 표시. 아직 응답이 없는 컴파트먼트는 `<cmp>  — loading —` 플레이스홀더 행으로 자리를 표시한다.
- 권한 없는 하위 컴파트먼트(401/404)는 무시하고 `⊕ +4 sub, 1 skipped` 로만 알린다.
- `/` 필터에 `cmp:app-a` 같은 컬럼 접두어 필터가 있으면 좋음.

### F4. 전체 테넌시 모드 (선택)
- `0` 으로 진입. 헤더 `Compartment:` 는 `ywdcloud ⊕ all`.
- OCI list API는 `compartmentId` 필수라 fan-out이 필요하므로, 리소스 타입별로 **Resource Search**
  (예: `query instance resources where lifecycleState = 'RUNNING'`) 를 사용해 테넌시 전체를 한 번에 가져온다.
- Search 응답은 필드가 제한적이므로 리스트는 Search로 채우고 상세 진입 시 개별 Get으로 보강한다.
- 지원되지 않는 리소스 타입은 모드 진입 시 명시적으로 "미지원" 안내.

### F5. 컴파트먼트 피커 (필수)
- **기존 Resources 피커 프레임을 그대로 복제**: 타이틀 `Compartments`, 우측 `매칭/전체` 카운트, `>` fuzzy 입력줄, 구분선, 하단 키 힌트.
- 내용은 플랫 목록이 아닌 **계층 트리**. 같은 이름의 컴파트먼트(`dev/app`, `prod/app`)가 흔하므로 경로 컨텍스트가 항상 보여야 한다. fuzzy 매칭 시 부모 노드는 접히지 않고 남겨 경로를 유지한다.
- 각 노드 우측에 `⊕ n` 으로 하위 컴파트먼트 수를 표시 → 서브트리를 켰을 때 fan-out 규모를 미리 알 수 있다. 선택 행(하이라이트) 위에서도 유지.
- 현재 컴파트먼트는 `●` 마커.
- 피커 하단에 선택 행의 **DESCRIPTION 한 줄** 표시. 개인 이름 컴파트먼트가 수십 개인 테넌시에서는 설명이 식별에 필수이며, 기존 Compartments 테이블의 설명 컬럼 역할을 여기서 흡수한다.
- 키: `Enter` 선택, `Tab` 선택 + 서브트리 ON, `p` pin/unpin, `Esc` 닫기.

### F6. Compartments 리소스 타입의 역할 변경
- 리소스 피커의 `Compartments` 항목은 유지한다(설명·상태 열람 용도).
- 그 테이블에서 `Enter` 는 **컨텍스트 설정이 아니라 상세 보기**로 바꾼다. 컨텍스트 전환은 어디서든 `c` 하나로 통일한다.
- 필요하면 그 테이블에 `c` 를 눌렀을 때 현재 커서 행이 피커에 미리 선택된 상태로 열리게 한다.

---

## 5. UI 스펙

### 5.1 헤더 / 푸터 (일반 모드)
```
Profile:     DEFAULT                                              TOCI!
Region:      ap-seoul-1                                           OCI TUI
Resource:    Exadata VM Clusters (Exascale)
Compartment: ywdcloud/juseok.oh/hub-and-spoke › vcn-db
Recent:      1 hub-and-spoke  2 vcn-app  3 kakaobank_poc  4 DATABASE     0 all
┌──────────────── Exadata VM Clusters (Exascale) [3] ────────────────┐
│ NAME              STATE      SHAPE    LICENSE  NODES  ECPU  DISK%   │
│ ExascaleRAC       Available  EXADBXS  BYOL     2      16    46%     │
│  ├─ rac-exascale2 Stopped                                          │
│  └─ rac-exascale1 Stopped                                          │
└────────────────────────────────────────────────────────────────────┘
3 items · space: shortcuts · c: compartment · C: subtree off · show nodes: on
```
- `Recent:` 는 5번째 헤더 라인. 숫자는 accent(노랑), 이름은 기본색, `0 all` 은 dim.
- `› vcn-db` 는 컴파트먼트가 아닌 하위 스코프이므로 dim 유지.

### 5.2 컴파트먼트 피커 (`c`)
```
┌──── Compartments ───────────────────────────── 4/37 ─┐
│ > j                                                   │
│ ─────────────────────────────────────────────────────  │
│ ywdcloud                                     ⊕ 37    │
│ › ├─ juseok.oh                               ⊕ 3  ◀  │   ← 선택 행 하이라이트
│   │  ├─ hub-and-spoke                        ●       │   ← 현재 컴파트먼트
│   │  ├─ vcn-app                                      │
│   │  └─ vcn-db                                       │
│   ├─ jg.jeon                                         │
│   └─ jinho.yoon                                      │
│ 클라우드컨설팅팀 오주석                                    │   ← 선택 행 DESCRIPTION
│ enter select · tab +subtree · p pin · esc            │
└───────────────────────────────────────────────────────┘
```
- 프레임·타이틀·카운트·입력줄은 Resources 피커와 동일한 스타일 함수를 사용한다.

### 5.3 서브트리 모드 ON (`Tab` 또는 `C`)
```
Resource:    Exadata VM Clusters (Exascale)
Compartment: ywdcloud/juseok.oh ⊕ +3 sub
Recent:      1 juseok.oh⊕  2 hub-and-spoke  3 vcn-app  4 kakaobank_poc     0 all
┌──── Exadata VM Clusters (Exascale) [5] ──────────── ↻ 3/4 loaded ─┐
│ COMPARTMENT    NAME              STATE      SHAPE    NODES         │
│ hub-and-spoke  ExascaleRAC       Available  EXADBXS  2             │
│                 ├─ rac-exascale2 Stopped                           │
│                 └─ rac-exascale1 Stopped                           │
│ vcn-app        AppExaVM          Available  EXADBXS  2             │
│ vcn-db         — loading —                                         │
└────────────────────────────────────────────────────────────────────┘
5 items · space: shortcuts · c: compartment · C: subtree on · show nodes: on
```
- 타이틀 바 우측 `↻ 3/4 loaded` 는 완료되면 사라진다.
- `COMPARTMENT` 컬럼은 accent 계열이되 채도를 낮춰 `NAME` 보다 뒤로 빠지게.

### 5.4 색상 규칙 (lipgloss)
- 헤더의 `⊕ +N sub` 와 푸터 `C: subtree on` 은 **같은 색**(현재 Active 에 쓰는 초록 계열)으로 묶어 모드 상태를 두 곳에서 동시에 전달.
- `Recent:` 숫자 키는 테이블 헤더에 쓰는 노랑 계열, `⊕ n` 도 동일.
- 피커의 `⊕ n` 은 선택 행 배경(초록) 위에서도 읽혀야 한다 — 기존 WCAG 대비 규칙 적용.
- `— loading —`, `0 all`, `› scope` 는 dim.

---

## 6. 구현 메모

### 상태 모델
```go
type Context struct {
    Profile     string
    Region      string
    Compartment CompartmentRef   // ID + 경로
    Subtree     bool
    Scope       *ScopeRef        // › vcn-db 같은 하위 필터, 컴파트먼트 전환 시 nil
    Recent      []RecentEntry    // MRU 최대 9, {CompartmentRef, Subtree, Pinned}
}
```
- `Context` 는 앱 루트 모델이 소유하고, 각 리소스 뷰는 읽기만 한다.
- 컨텍스트 변경은 `ContextChangedMsg` 로 브로드캐스트 → 현재 뷰가 재조회.
- 헤더 컴포넌트는 `Context` 만 보고 5줄을 렌더한다.

### 컴파트먼트 트리
- 앱 시작 시 `ListCompartments(compartmentIdInSubtree=true, accessLevel=ACCESSIBLE)` 한 번으로 전체 트리(이름·설명·상태·부모)를 받아 캐시. 피커·경로 계산·`⊕ n` 은 캐시로만 처리.
- 수동 리프레시 키 제공. Compartments 리소스 뷰(F6)도 같은 캐시를 읽는다.

### 피커
- 기존 Resources 피커 모델을 일반화해 `Picker[T]` 로 만들고, 행 렌더러만 트리용으로 주입한다. 프레임/입력/카운트 로직을 공유해야 스타일이 어긋나지 않는다.
- fuzzy 매칭은 노드 이름과 전체 경로 양쪽에 대해 수행. 매칭된 노드의 조상은 항상 표시.

### 서브트리 fan-out
- `errgroup` + 세마포어(동시 5~8)로 rate limit 방어.
- 컴파트먼트별 결과를 채널로 받아 도착 순서대로 테이블 append. 미도착 컴파트먼트는 플레이스홀더 행. 전체 완료 후 한 번 재정렬.
- 401/404 는 skip 카운트만 증가, 그 외 에러는 헤더에 요약 표시.
- 취소 가능해야 함(`Esc` 또는 컨텍스트 재변경 시 이전 fan-out 취소 — `context.WithCancel`).

### Bubble Tea 측 고려
- 오버레이는 bubbletea/lipgloss v2 의 Layer/Canvas 컴포지팅으로 구현한다(v1이면 이 기능을 계기로 v2 마이그레이션).
- 키 라우팅: 오버레이 최상단 → 포커스 컴포넌트 순. `c`/`C`/`0-9` 는 루트 모델이 가로채는 전역 키.

---

## 7. 우선순위 및 범위

| 단계 | 항목 | 비고 |
|---|---|---|
| P0 | F1 뷰 내 스위칭, F5 트리 피커, F2 Recent 핫키, F6 Compartments 뷰 역할 변경 | 사용자 피드백 직접 해소 |
| P1 | F3 서브트리 모드 | OCI 특성상 곧 다시 나올 요청, 설계는 P0 와 함께 |
| P2 | F4 전체 테넌시 | Resource Search 의존, 리소스 타입별 점진 지원 |

---

## 8. 수용 기준

- [ ] Exadata VM Clusters 뷰에서 `c` → 다른 컴파트먼트 선택 → 여전히 같은 리소스 뷰이며 목록만 교체되고, `› scope` 는 초기화된다.
- [ ] 헤더 `Recent:` 라인에 최근 컴파트먼트가 번호와 함께 보이고, `1`~`9` 로 즉시 전환되며 재시작 후에도 유지된다.
- [ ] `C` 토글 시 헤더에 `⊕ +N sub`, 푸터에 `C: subtree on`, 테이블에 `COMPARTMENT` 컬럼이 나타나고, 첫 결과가 전체 완료 전에 표시된다.
- [ ] 피커가 Resources 피커와 같은 프레임을 쓰고, 동명 컴파트먼트를 경로로 구분할 수 있으며, 하단에 선택 행의 설명이 보인다.
- [ ] `Tab` 으로 서브트리 모드와 함께 선택된다.
- [ ] 권한 없는 하위 컴파트먼트가 있어도 조회가 실패하지 않고 skip 수가 표시된다.
- [ ] Compartments 리소스 뷰에서 `Enter` 는 상세 보기이며 컨텍스트를 바꾸지 않는다.
