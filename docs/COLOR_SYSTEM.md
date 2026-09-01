# 컬러 시스템

toci는 웹 프로젝트가 아니라 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 기반 터미널 UI(TUI)라서, Tailwind/CSS 변수 같은 디자인 토큰 파일은 없다. 대신 [Lipgloss](https://github.com/charmbracelet/lipgloss) 스타일 변수들이 그 역할을 한다. `lipgloss.Color()`는 ANSI 256색 인덱스 문자열(`"245"`)과 트루컬러 헥스 문자열(`"#689878"`)을 둘 다 받는데, toci는 두 형태를 섞어 쓴다 — 트루컬러 미지원 터미널에서는 lipgloss가 알아서 가장 가까운 색으로 다운샘플한다.

## OCI 그린 팔레트 (`internal/app/model.go`)

메인 UI(헤더, 테이블, 테두리, 도움말)의 액센트/크롬 컬러는 OCI 콘솔 사이드바를 캡처한 스크린샷에서 실측한 7개 색 중 6개를 골라 쓴다 — 임의로 고른 브랜드 컬러가 아니라 픽셀 비중 순으로 뽑은 실제 값이다.

| 상수 | 값 | 용도 |
| --- | --- | --- |
| `ociAccent` | `#689878` (가장 많이 나온 세이지, 29%) | 제목/강조 텍스트, 테이블 헤더, 피커·박스 제목 |
| `ociBorder` | `#487858` (중간 포레스트) | 모든 테두리 — 박스, 테이블, 도움말 팝업, Security List 테이블 |
| `ociMuted` | `#588868` | 상태줄, 진행 바 빈 트랙 등 보조 상태 텍스트 |
| `ociSubtle` | `#88b898` (가장 밝음) | 라벨 접두어(`Profile:` 등), 도움말 설명 텍스트 |
| `ociSelBg` | `#386848` (가장 어두움) | 테이블/피커에서 선택된 행의 배경 |
| `ociHighlt` | `#e8c878` (베이지/골드 포인트) | 도움말 키 라벨 강조 |

**의도적으로 이 팔레트에 넣지 않은 것**: `successStyle`(`2`, 초록)·`errorStyle`(`196`, 빨강)과 `state_color.go`의 RUNNING/STOPPED 배지는 상태를 의미하는 신호색이라, 그린 테마로 바꾸면 "빨강=중지/에러"라는 신호 자체가 모호해진다. 그대로 뒀다.

## 공통 스타일 (`internal/app/model.go`)

| 변수 | 색 | 용도 |
| --- | --- | --- |
| `titleStyle` | `ociAccent` + Bold | 테이블 헤더, 피커/박스 제목 등 강조 텍스트 (헤더 값 자체는 아래 `headerValueStyle` 참고) |
| `pathStyle` | `ociSubtle` | 라벨 접두어, 보조 텍스트 |
| `statusStyle` | `ociMuted` | 상태줄, 필터 힌트, 로딩 텍스트 |
| `boxStyle` | 테두리 `ociBorder` | 확인/프롬프트 박스 |
| `selStyle` | fg `255`(흰색) / bg `ociSelBg` + Bold | 테이블·피커에서 커서가 위치한 선택 행 |
| `headerValueStyle` | `255`(흰색) + Bold | `Profile:`/`Region:`/`Resource:`/`Compartment:` 값과 코너 버전 문구 전용 — `titleStyle`과 분리해서, 이 값들만 흰색으로 바꿔도 테이블 헤더 등 다른 곳은 그대로 `ociAccent`를 유지한다 |
| `successStyle` | `2`(초록) | 성공 메시지 (의미색, 팔레트 예외) |
| `errorStyle` | `196`(빨강) + Bold | 에러 메시지 (의미색, 팔레트 예외) |

테이블 데이터 셀 텍스트는 `internal/app/state_color.go`의 `whitenDataRows`가 렌더 후처리로 흰색(`255`)을 입힌다 — bubbles 테이블의 기본 Cell 스타일에는 전경색이 없어서 터미널 기본색(어두운 흰색)으로 보이던 걸 고친 것. `table.Styles.Cell`에 직접 `Foreground`를 주는 방식은 셀마다 개별 리셋 코드가 생겨서 **선택 행의 배경색이 첫 컬럼 이후로 끊기는** 버그가 났다(실측 확인) — 그래서 셀 단위가 아니라 **행 전체를 한 번에** 감싸는 방식으로 구현했고, 이미 흰색인 선택 행(`selStyle`)과 헤더 행은 건너뛴다. `colorizeInstanceState`(STATE 배지)보다 먼저 실행해서, 배지 뒤 컬럼도 흰색이 이어지도록 순서를 맞췄다.

## 화면별 적용 예: 스플래시 (`internal/app/splash.go`)

스플래시는 메인 UI 팔레트를 전혀 안 쓴다 — `titleStyle`/`statusStyle`/`pathStyle`을 앞으로 또 바꾸더라도 시작 화면 룩이 같이 안 바뀌도록, 처음부터 전용 스타일을 따로 둔다.

| 요소 | 스타일 | 값 |
| --- | --- | --- |
| ASCII 로고, 진행 바 채워진 부분 | `splashLogoStyle` | `196`(OCI 브랜드 레드) + Bold |
| 서브타이틀 / 프로필 / 빈 트랙 / 로딩 문구 | `splashMutedStyle` / `splashProfileStyle` | `241` / `245` — 메인 UI가 예전에 쓰던 값을 그대로 복사해 고정 |
| 스피너 아이콘 | `spinnerStyle` | `220`(노랑) — taws의 스피너 색을 그대로 |

우측 상단 코너 워드마크(`cornerLogo`, `internal/app/model.go`)도 `splashLogoStyle`을 그대로 써서 `196` 레드로 고정되어 있다 — 스플래시 로고와 동일 계열이라는 의도. 그 바로 아래 버전/`OCI TUI` 문구만 `headerValueStyle`(흰색)이라 메인 UI 톤을 따라간다.

## 리소스 상태 배지 (`internal/app/state_color.go`)

인스턴스 테이블의 STATE 컬럼에 RUNNING/STOPPED를 렌더링 후 후처리로 색칠하는 배지 스타일. 전경색은 기본 색상 `0`이 아니라 256색 인덱스 `16`(순수 검정)을 쓰는데, `Bold + 기본 0-7번 전경색` 조합을 밝은 변형으로 바꿔버리는 터미널들이 있어서다.

| 상태 | 배경색 | 선택된 행일 때 배경색 |
| --- | --- | --- |
| RUNNING | `2` (녹색) | `28` (짙은 녹색) |
| STOPPED | `1` (빨강) | `88` (짙은 빨강) |

## 도움말 팝업 (`internal/app/help.go`)

LazyVim 스타일 which-key 오버레이 전용 색 — 위 OCI 팔레트에서 가져온다.

| 변수 | 값 | 용도 |
| --- | --- | --- |
| `helpKeyStyle` | `ociHighlt`(베이지/골드) + Bold | 키 바인딩 라벨 |
| `helpDescStyle` | `ociSubtle` | 키 바인딩 설명 |

## 테두리 (borders)

박스/테이블 테두리는 전부 `ociBorder`(`#487858`)로 통일했다 — help box, 확인/프롬프트 박스(`boxStyle`), `f` 리소스 검색 박스, 리소스 테이블을 감싸는 박스(`renderTableBox`), Security List 규칙 테이블이 모두 같은 값을 공유한다. 예전엔 무채색 회색(`240`)과 액센트 파랑(`39`)이 용도별로 나뉘어 있었지만, 그린 테마로 옮기면서 하나로 합쳤다.

## 색상 선택 원칙

- **새 색을 추가하기 전에 위 표에서 의미가 맞는 기존 스타일/상수를 재사용한다.** 예: 보조 텍스트는 항상 `pathStyle`(`ociSubtle`), 테두리는 항상 `ociBorder`.
- 의미를 전달하는 색(성공/에러/RUNNING/STOPPED)은 액센트 팔레트에 넣지 않는다 — 상태 신호와 브랜드 장식은 분리해서 유지한다.
- 배경색 위에 텍스트를 올릴 때(선택 행, 상태 배지) 전경색은 기본 `0`이 아니라 `16`(검정) 또는 `255`(흰색) 같은 256색 인덱스를 쓴다. `0`/`7` 같은 기본 색은 Bold와 결합했을 때 일부 터미널에서 밝게 반전된다.
- 여러 컬럼/셀에 걸쳐 색을 입힐 땐 셀 단위로 각각 스타일을 적용하지 말 것 — 중간에 리셋 코드가 끼어들어 바깥(선택 행 등)의 스타일을 끊어버릴 수 있다. `whitenDataRows`/`colorizeInstanceState`처럼 행 전체를 감싸거나, `ansi.Cut` 기반으로 "이미 열려있는 스타일을 이어받는" 방식(`embedInLine`, `spliceOverlay`)을 쓴다.
- 선택 상태 강조는 배경색을 한 단계 어둡게(`2`→`28`, `1`→`88`) 바꾸는 방식을 따른다. 새로운 배지를 추가할 때도 이 패턴을 유지한다.
- 다크/라이트 모드 분기 코드는 만들지 않는다 — 사용자 터미널 컬러스킴이 이미 그 역할을 한다.
