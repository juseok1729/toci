# 컬러 시스템

toci는 웹 프로젝트가 아니라 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 기반 터미널 UI(TUI)라서, Tailwind/CSS 변수 같은 디자인 토큰 파일은 없다. 대신 [Lipgloss](https://github.com/charmbracelet/lipgloss) 스타일 변수들이 그 역할을 하며, 전부 **ANSI 256색 팔레트 인덱스**(`lipgloss.Color("39")` 형태의 문자열)로 지정되어 있다. 실제 RGB 값은 터미널 에뮬레이터의 컬러스킴이 결정하므로, 다크/라이트 모드는 앱이 아니라 사용자 터미널 설정에 위임된다.

## 공통 스타일 (`internal/app/model.go:23-31`)

앱 전역에서 재사용되는 기본 스타일. 대부분의 화면(sidebar, splash, help 등)이 새 색을 정의하지 않고 이 스타일들을 그대로 가져다 쓴다.

| 변수 | ANSI 코드 | 용도 |
| --- | --- | --- |
| `titleStyle` | `39` (파랑) + Bold | 제목, 테이블 헤더, 박스 테두리, 강조 텍스트 |
| `pathStyle` | `245` (연회색) | 경로, 서브타이틀 등 보조 텍스트 |
| `statusStyle` | `241` (진회색) | 상태줄, 진행 바의 빈 트랙 |
| `successStyle` | `2` (녹색) | 성공 메시지 |
| `errorStyle` | `196` (밝은 빨강) + Bold | 에러 메시지 |
| `selStyle` | fg `0` / bg `39` | 리스트/테이블에서 커서가 위치한 선택 행 |
| `boxStyle` | 테두리 `39` | 라운드 보더 박스 |

## 화면별 적용 예: 스플래시 (`internal/app/splash.go`)

시작 화면은 새 색을 정의하지 않고 위 공통 스타일을 요소별로 그대로 갖다 쓴다.

| 요소 | 스타일 |
| --- | --- |
| ASCII 로고 (`TOCI`) | `titleStyle` |
| 서브타이틀 / 스피너 상태 텍스트 | `statusStyle` |
| 프로필 이름 | `pathStyle` |
| 진행 바 — 채워진 부분 | `titleStyle` |
| 진행 바 — 빈 트랙(`░`) | `statusStyle` |

## 리소스 상태 배지 (`internal/app/state_color.go:11-21`)

인스턴스 테이블의 STATE 컬럼에 RUNNING/STOPPED를 렌더링 후 후처리로 색칠하는 배지 스타일. 전경색은 기본 색상 `0`이 아니라 256색 인덱스 `16`(순수 검정)을 쓰는데, `Bold + 기본 0-7번 전경색` 조합을 밝은 변형으로 바꿔버리는 터미널들이 있어서다.

| 상태 | 배경색 | 선택된 행일 때 배경색 |
| --- | --- | --- |
| RUNNING | `2` (녹색) | `28` (짙은 녹색) |
| STOPPED | `1` (빨강) | `88` (짙은 빨강) |

## 도움말 팝업 (`internal/app/help.go:17-18`)

LazyVim 스타일 which-key 오버레이 전용 색.

| 변수 | ANSI 코드 | 용도 |
| --- | --- | --- |
| `helpKeyStyle` | `212` (분홍/마젠타) + Bold | 키 바인딩 라벨 |
| `helpDescStyle` | `250` (밝은 회색) | 키 바인딩 설명 |

## 테두리 (borders)

박스/테이블 테두리에는 공통적으로 `240`(무채색 회색)을 쓴다 — help box(`help.go:47`), sidebar(`sidebar.go:236`), security list rules 테이블(`security_rules.go:111`)이 모두 동일한 값을 공유한다. 강조가 필요한 박스(`boxStyle`, 테이블 헤더, `f` 리소스 검색 박스, 리소스 테이블을 감싸는 박스 `renderTableBox`)만 예외적으로 `39`(파랑)를 쓴다.

## 색상 선택 원칙

- **새 색을 추가하기 전에 위 표에서 의미가 맞는 기존 스타일을 재사용한다.** 예: 보조 텍스트는 항상 `pathStyle`(245), 테두리는 항상 `240`.
- 상태를 배경색으로 표현할 때(RUNNING/STOPPED 배지처럼) 전경색은 `16`을 쓴다. `0`을 쓰면 Bold와 결합했을 때 일부 터미널에서 밝게 반전된다.
- 선택 상태 강조는 배경색을 한 단계 어둡게(`2`→`28`, `1`→`88`) 바꾸는 방식을 따른다. 새로운 배지를 추가할 때도 이 패턴을 유지한다.
- 다크/라이트 모드 분기 코드는 만들지 않는다 — 사용자 터미널 컬러스킴이 이미 그 역할을 한다.
