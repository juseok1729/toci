# toci — 작업 기록

버전(태그)별 변경사항. 배경/이유가 코드만 봐서는 안 드러나는 결정 위주로 기록.

## v0.1.12

### 커서(선택된 행)에서 EDITION 색상이 안 보이는 문제 → 파스텔로 정착

- v0.1.11에서 EDITION 색을 "강렬하게" 요청받아 비비드 컬러로 바꿨더니, 선택된 행(초록 하이라이트, `ociSelBg` `#386848`)에 올라가면 잘 안 보인다는 피드백 — 특히 `EE-DEV`의 초록은 배경과 색 계열 자체가 겹쳐서(hue collision) 최악.
- 시도 1: 선택된 셀만 검정 배경(배지처럼) → 하이라이트 바 중간이 뚝 끊긴 것처럼 보여서 롤백.
- 시도 2: 같은 색상의 어두운 톤("형광펜 위 잉크"처럼) → 사용자가 직접 픽스한 방향으로, 채도 있는 색끼리 부딪히는 건 해결됐지만 요청으로 다시 롤백.
- **최종**: 각 티어 색의 **파스텔(연한) 톤**으로 선택 행 전용 색을 지정 — SE2 `#7DF9FF`, EE `#AFD7FF`, EE-HP `#E2C4FF`, EE-EP `#FFB8DE`, EE-DEV는 우연히 기존 `ociHighlt`(`#e8c878`, 골드)와 같은 값이라 그 상수를 그대로 재사용. 밝기 차이로 대비를 주는 방식이라 채도 경쟁 없이 배경과 안 부딪힘.
- 참고로 `ociSelBg`(`#386848`)를 "짙은 초록"이라고 불렀었는데, 실제 HSL 밝기는 약 31%로 진짜 다크 톤이 아니라 muted한 중간 톤에 가깝다 — 색 충돌은 밝기보다 채도 경쟁 때문이었음.

## v0.1.11

### DB System EDITION 축약어 + 색상, VERSION 컬럼 추가

- **EDITION 축약**: `ENTERPRISE_EDITION_EXTREME_PERFORMANCE` 같은 원본 enum이 너무 길어서, Oracle 라이선싱 문서/업계에서 실제 쓰는 축약어로 변환(`dbEditionAbbrev`) — `STANDARD_EDITION`→`SE2`, `ENTERPRISE_EDITION`→`EE`, `..._HIGH_PERFORMANCE`→`EE-HP`, `..._EXTREME_PERFORMANCE`→`EE-EP`, `..._DEVELOPER`→`EE-DEV`(라이선스 비용 없는 개발/테스트 전용 에디션). 컬럼 폭도 39→6으로 축소.
- **EDITION 색상**: STATE/NODE의 Good/Bad/Warn과는 다른 축(에디션은 상태/헬스 신호가 아니라 티어 구분)이라 `colorizeState`에 얹지 않고 별도 `colorizeEdition` 패스로 분리. 정확히 일치(`==`)로 매칭 — `EE`가 `EE-HP`/`EE-EP`/`EE-DEV`의 접두어라 `Contains`였으면 오작동했을 부분.
- **VERSION 컬럼**: `DbSystemSummary.Version`이 `ListDbSystems`에 이미 들어있어서(추가 API 호출 불필요) 바로 노출 — MEM(GB)처럼 `GetDbSystem`이 따로 필요한 필드와 달리 공짜.

## v0.1.10

### DB Node / Data Guard 가시성 추가 (DB System, Exadata VM Cluster)

- **문제**: DB System의 STATE(`Available`)는 DbSystem 리소스 자체 상태일 뿐, 그 안에서 실제 DB를 돌리는 DB Node는 별도의 LifecycleState를 가진다 — 노드만 따로 정지시켜도 DbSystem은 계속 `Available`로 보임. 실제로 DEFAULT 테넌시의 한 테스트 컴파트먼트에서 DB System 10개 전부가 STATE=Available인데 NODE는 전부 Stopped인 걸 실측으로 확인.
- **NODE 컬럼** 추가 — `ListDbNodes`로 각 노드의 실제 상태를 조회. 이 API는 `DbSystemId`나 `VmClusterId` 둘 중 하나가 반드시 있어야 하는데(SDK 구조체 태그는 둘 다 `mandatory:false`로만 표기되어 있어 실제로 호출해보고서야 발견 — 없으면 400 `MissingParameter`), DB System/Exadata VM Cluster 당 한 번씩 호출(컴파트먼트당 리소스 수가 적어서 비용 문제 없음). 2노드 RAC처럼 값이 여러 개면 `/`로 join.
- **ROLE 컬럼** 추가 — Primary/Standby/RAC 구분. 2노드 이상이면 무조건 RAC(OCI에서 노드 여러 개인 VM DB System은 RAC 말고는 존재할 수 없음), 아니면 Data Guard 역할(`ListDatabases` → `ListDataGuardAssociations`, DB System당 최대 2콜 추가)을 보여주고 둘 다 없으면 `-`.
- **크로스리전 Data Guard 힌트**: Standby가 다른 리전에 있으면 그 리전 목록엔 애초에 안 뜨는데(리소스 목록 조회 자체가 리전 스코프), Primary 쪽에서라도 상대가 어디 있는지 알 수 있게 `Primary→ap-tokyo-1`처럼 표시. 추가 API 호출 없이 `PeerDbSystemId`(OCID 안에 리전이 그대로 박혀있음, 예: `ocid1.dbsystem.oc1.ap-tokyo-1.xxxx`)를 파싱해서 얻음.
- **MEM(GB)/DISK(GB) 컬럼**도 같이 추가 — `MemorySizeInGBs`는 `ListDbSystems`엔 항상 nil이고 `GetDbSystem`에서만 실제 값이 나옴(실측 확인). `DataStorageSizeInGBs`(DATA)+`RecoStorageSizeInGB`(RECO)는 반대로 `ListDbSystems`에 이미 들어있어서 추가 호출 불필요.
- `colorizeState`를 STATE 전용에서 임의 컬럼(제목 파라미터) + 셀 값을 `/` 기준으로 쪼개 **부분별로 독립 색칠**하도록 일반화 — `Available/Stopped`가 통째로 한 색이 아니라 초록/빨강으로 따로 칠해져서, 여러 노드 중 어느 게 문제인지 한눈에 보임.
- **회귀 수정**: `Row.Raw`를 원본 SDK 구조체에서 `DbSystemRow`/`CloudVmClusterRow` 래퍼로 바꾸면서, Mermaid 다이어그램 export(`internal/app/diagram.go`)의 타입 단언이 조용히 실패할 뻔한 걸 미리 전체 코드베이스를 훑어서 잡아 고침.
- **버그 수정**: ROLE/NODE 컬럼의 `Width`(예: 10, 20)가 `fitColumnWidth`에서 힌트가 아니라 **하드 상한선**으로 쓰이는 걸 놓쳐서, 긴 값(`Standby→af-johannesburg-1` 같은 크로스리전 힌트, 다수 노드 Exadata)이 잘릴 뻔했음 — 각각 실제 최악값 기준으로 재계산해서 수정(ROLE/DB System NODE: 25, Exadata NODE: 103). 회귀 테스트로 이 패턴 고정.

## v0.1.8

### STATE 컬럼 재배치 + 배지 → 텍스트 색상 전환

- STATE를 NAME 바로 다음 컬럼으로 이동 — 이전엔 각 리소스마다 STATE가 맨 끝이라, 어떤 리소스든 이름 보고 바로 다음에 상태를 확인하려면 옆으로 스크롤 없이도 눈이 왔다 갔다 해야 했음. `drg`/`nsg`는 원래도 NAME 다음이라 그대로 뒀고, 나머지 10개 리소스 파일만 컬럼 순서를 바꿈.
- STATE 값을 배경색 배지(`stateBgRunning` 등, `Background`로 칠하던 방식)에서 **텍스트 색만** 칠하는 방식(`stateTextGood` 등)으로 전환 — 일반 행은 다른 컬럼처럼 배경 없이 텍스트만 색이 있어서 테이블이 덜 시끄러워짐. 선택된 행에서는 텍스트 색 스타일에 `Background(ociSelBg)`를 별도로 얹은 `*Selected` 변형을 써서, `style.Render()`가 만드는 리셋 코드가 선택 행 하이라이트 중간에 구멍을 내는 걸 막음(`whitenDataRows`/`colorizeInstanceState`(현 `colorizeState`) 개발 때 이미 겪었던 것과 같은 종류의 함정).

### 상태값 표시를 Title Case로 (`Running`, `Needs Attention`)

- OCI SDK가 주는 전부 대문자 enum 값(`RUNNING`, `NEEDS_ATTENTION`)을 그대로 보여주지 않고, `internal/registry/resource.go`의 제네릭 헬퍼 `stateLabel[T ~string]`이 언더스코어 기준으로 단어를 쪼개 각 단어 첫 글자만 대문자로 바꿔 표시 — `NEEDS_ATTENTION` → `Needs Attention`. 리소스 12종의 `LifecycleState` 타입이 전부 다른 SDK enum 타입이라 제네릭으로 만들어서 타입 단언 없이 공용으로 씀.
- 색칠 로직(`colorizeState`)의 매칭 문자열도 이 Title Case 결과에 맞춰 `"RUNNING"` → `"Running"` 등으로 같이 바꿈 — 안 바꾸면 대소문자가 안 맞아 색이 전혀 안 붙는다.

### STATE 색상을 전체 12종 리소스로 확장 + 실패/주의 등급 추가

- 기존엔 Instance 리소스에서만, RUNNING(초록)/STOPPED(빨강) 두 값만 색이 붙었음. 이번에 `colorizeInstanceState`를 `colorizeState`로 일반화해서 **모든 리소스 종류의 STATE 컬럼**에 적용되도록 `View()`의 `if current().Key() == "instance"` 게이트를 제거.
- 3단계 색상 등급 도입 — Good(초록: Running/Active/Available/Standby), Bad(빨강: Stopped/Failed/Inaccessible/Unavailable), Warn(노랑: Needs Attention). 판정은 **Warn → Bad → Good** 순으로 고정 — ADB의 `Available Needs Attention`처럼 여러 등급의 단어를 동시에 포함하는 값이 있어서, 순서를 안 지키면 "Available"이 먼저 걸려 Good으로 잘못 칠해짐.
- Provisioning/Terminated/Updating 같은 전환·종료 상태는 의도적으로 색을 안 붙임 — 신호가 뚜렷한 성공/실패/주의만 색으로 강조하고 나머지까지 칠하면 오히려 신호가 흐려짐.
- 리소스별로 실제 SDK가 갖는 `LifecycleState` 값 전체와 등급 매핑을 `docs/COLOR_SYSTEM.md`에 표로 정리해둠.

### 문서

- README(영/한) Screenshots 섹션 이미지를 최신 UI(STATE 위치, 색상 텍스트) 반영한 캡처로 교체.
- README(영/한)의 "RUNNING/STOPPED 배지" 문구를 위 변경사항에 맞게 갱신.

## v0.1.4

### 사이드바 트리 제거, VCN 선택 시 검색창 직결

- `internal/app/sidebar.go` 통째로 삭제 — `t`/`modeSidebar`/트리 렌더링 전부 제거. `f`(리소스 검색)가 생기고 나니 상시 표시되는 트리 패널이 중복 기능이 됨.
- VCN을 고르면(`i` 또는 `Enter`) 예전엔 사이드바로 포커스가 넘어갔는데, 이제 곧바로 리소스 검색창(`f`)이 뜬다 — `selectVcnFilter`가 `openResourceSearch()`를 직접 호출.
- "Compartments"를 검색창에서 고르면 테넌시 루트로 리셋하는 동작(`switchToRootCompartments`)은 예전에 사이드바 전용이었는데, 이제 `pickerResource` 처리 쪽으로 옮겨서 그대로 유지됨 — 안 옮겼으면 리프 컴파트먼트에 있을 때 빈 화면만 반복되는 회귀가 났을 것.

### OCI 그린 팔레트로 리테마

- OCI 콘솔 사이드바를 캡처한 스크린샷에서 실제 픽셀 색상을 추출(비중순 7색) → 6색을 골라 `internal/app/model.go`의 `ociAccent`/`ociBorder`/`ociMuted`/`ociSubtle`/`ociSelBg`/`ociHighlt` 상수로 도입. 자세한 매핑은 `docs/COLOR_SYSTEM.md` 참고.
- 성공/에러/RUNNING/STOPPED 배지 같은 "의미 신호" 색은 그대로 뒀다 — 빨강을 초록으로 바꾸면 의미가 헷갈림.
- 스플래시 화면은 전용 스타일(`splashLogoStyle`/`splashMutedStyle`/`splashProfileStyle`)로 분리해서 메인 UI 팔레트 변경에 영향받지 않게 함. 우측 상단 코너 워드마크도 `splashLogoStyle`(레드) 그대로.
- `Profile:`/`Region:`/`Resource:`/`Compartment:` 값과 코너 버전 문구만 `headerValueStyle`(흰색)로 분리 — 나머지 `titleStyle` 사용처(테이블 헤더 등)는 그대로 액센트 그린 유지.
- 테이블 데이터 셀 텍스트도 흰색으로 — `state_color.go`의 `whitenDataRows`. 처음엔 `table.Styles.Cell`에 직접 `Foreground`를 줬는데, **선택 행 배경이 첫 컬럼 이후로 끊기는** 버그가 나서(실제 tty로 pty+pyte 띄워서 픽셀 단위로 확인) 되돌리고, 행 전체를 한 번에 감싸는 방식 + `colorizeInstanceState`보다 먼저 실행하는 순서로 다시 구현.

### 버그 수정

- **스페이스바 도움말 토글**: 팝업이 열린 상태에서 스페이스바를 다시 누르면, "아무 키나 누르면 닫힘" 처리가 먼저 닫은 걸 스페이스바 자체의 토글 로직이 곧바로 다시 열어버려서 사실상 안 닫히던 버그. `wasHelpOpen`으로 원래 상태를 기억해두고 판단하도록 수정.
- **검색창(`f`) 뜰 때 테이블 우측 테두리 소실**: `overlayCenter`가 쓰는 `spliceOverlay`가 코너 전용(도움말 팝업, 코너 로고)으로 설계돼서, 박스 뒤에 남는 원래 내용을 버리고 있었다 — 코너 오버레이는 어차피 끝까지 덮으니 안 보였는데, 폭이 좁은 센터 오버레이(리소스 검색창)는 박스 뒤에 실제 콘텐츠(테이블 우측 테두리 등)가 남아있어서 문제가 드러남. `embedInLine`과 같은 left+box+right 3분할 스플라이스로 통일해서 해결.

### 기타

- 우측 상단에 작은 3행 블록 폰트 로고(`cornerLogoArt`) + 그 아래 릴리즈 버전(또는 `dev` 빌드면 "OCI TUI") 문구 추가. `cmd/toci/main.go`의 기존 `version`(ldflags 주입) 변수를 `app.New()`까지 연결.
- 스플래시 진행 바: 폭 확장, 10%→30%→60% 중 처음 두 단계를 빠르게, taws(github.com/huseyinbabal/taws) 스타일 브레일 스피너 + 랜덤 문구(단계 바뀔 때마다 재추첨), 세로 위치를 화면 위쪽으로 이동.
- `Profile:`/`Region:`/`Resource:` 아래 `Compartment:` 값도 별도 줄로 명시적 표시.

## v0.1.2

### 리소스 검색 (`f`) 및 사이드바 트리 접근 변경

- **`f`**: 리소스 종류를 퍼지 검색하는 창이 화면 상단 1/3 지점에 센터로 뜬다(`internal/app/model.go`의 `renderResourceSearch`/`overlayCenter`). 폭은 터미널의 60%(최소 50), 제목·매치 카운트(`n/m`)를 테두리에 박아넣는 Telescope 스타일. 기존 `picker`(region/action/bastion 피커) 인프라를 재사용.
- **`:`**: 더 이상 사이드바를 강제로 열지 않는다 — `t`로 트리가 이미 보일 때만 포커스 이동, 숨겨진 상태면 no-op (vim 폴더트리처럼 "먼저 열고 그다음 포커스"). `i`로 VCN 필터 진입 시 자동으로 트리를 여는 동작(`selectVcnFilter`)은 별개라 그대로 둠.
- 재사용 중 발견한 기존 버그 2개도 같이 고침: 좁은/0폭 터미널에서 `strings.Repeat` 음수 카운트 패닉, `textinput` 기본 프롬프트(`"> "`)와 직접 그린 `"> "`가 겹쳐 `> >`로 두 번 나오던 것(공유 생성자 `newPicker`에서 수정 — region/action/bastion 피커도 같이 고쳐짐).

### `d`(상세보기) / `Enter`(컨텍스트 액션) 분리

- `d`: 모든 리소스 종류에서 선택한 행의 상세(YAML)를 본다.
- `Enter`: Compartment는 하위 진입(기존과 동일), VCN 행에서는 `i`와 동일하게 VCN 필터를 선택, 그 외 리소스는 동작 없음. 예전엔 Enter가 컴파트먼트가 아니면 무조건 상세보기를 열었음.

### 리소스 테이블에 박스 테두리 + 컬럼 비례 조정

- 테이블 전체를 라운드 테두리 박스로 감싸고(`renderTableBox`), 상단 테두리 중앙에 `<리소스명> [<개수>]`를 박아넣음(액센트 블루 `39`).
- `fitColumns`: 화면이 컬럼들의 실제 필요 폭보다 넓을 때 남는 폭을 전부 특정 컬럼(마지막 컬럼 등) 하나에 몰아주던 방식을, 폭이 좁을 때 이미 쓰던 비례 축소와 동일하게 **비례 확대**로 통일. 이전 방식은 Instance 테이블에서 STATE가 마지막 컬럼이라 `colorizeInstanceState`가 칠하는 배지 배경까지 통째로 늘어나는 부작용이 있었음 — 이제 모든 컬럼이 같은 비율로 커진다. 정수 반올림으로 남는 오차(최대 컬럼 수만큼)만 마지막 컬럼에 보정해서 선택 행 하이라이트가 박스 오른쪽 끝까지 정확히 닿는 건 유지.
- 사이드바(`t`)를 숨겼을 때 테이블 박스가 이전엔 사이드바가 있을 때 폭 그대로 남아있었음(`relayout()`이 폭만 바꾸고 컬럼을 다시 계산하는 `refreshTable`은 안 불렀기 때문) — `relayout()`이 이제 컬럼만 제자리에서 다시 계산하는 `relayoutTableColumns()`를 항상 호출. `refreshTable`과 달리 테이블을 통째로 새로 만들지 않아 **커서 위치도 유지**됨.
- 사이드바 숨김 시 좌우 여백 대칭화: 박스에 좌우 1칸 패딩 추가, 사이드바 자리에 좌우 동일한 빈 여백 블록을 넣음 (`tableBoxOverhead` 2→4).

### 시작 스플래시 화면 진행 바

- 프로그레스바 폭 40→70, 최소 유지 시간 늘림(현재 `splashTicksPerStage=7`, 총 3단계 × 7틱 × 60ms ≈ 1.26초).
- 매 틱 조금씩 증가하던 방식 대신 단계별로 점프(현재 10% → 30% → 60%, 데이터 준비되면 바로 100%)하도록 변경 — 실제 신호는 초기 리소스 로딩 1건뿐이라 "살아있어 보이게" 하기 위한 연출.

### 문서

- `docs/COLOR_SYSTEM.md` 신규 — Lipgloss 기반 ANSI 256색 스타일 변수들을 역할별(공통 스타일/상태 배지/도움말/테두리)로 정리.
- README(영/한) 키 바인딩 표를 위 변경사항에 맞게 갱신.

## (초기 릴리즈)

## 왼쪽 사이드바 리소스 트리 (`internal/app/sidebar.go`)

`:` 를 눌렀을 때 뜨던 오버레이 피커를 없애고, 항상 보이는 왼쪽 사이드바 트리로 교체.

- `:` 는 이제 팝업을 여는 대신 사이드바로 포커스를 옮긴다(`modeSidebar`) — `j`/`k`·↑↓로 트리 이동, `Enter` 선택, `Esc` 취소. 팝업이 아니라 상시 표시되는 패널이라 현재 리소스가 항상 하이라이트되어 보인다.
- 리소스 종류에 category 필드가 없어서, 사이드바 전용으로 4개 카테고리(Compartments/Compute/Network/Load Balancing)를 `resourceCategories`에 하드코딩 — 리소스가 늘어나 분류가 안 맞으면 이 슬라이스만 고치면 됨.
- 기존 `pickerResource`(리소스 전용 피커)는 통째로 제거 — region/action/bastion 피커는 여전히 팝업 방식 그대로 둠 (자주 안 쓰는 짧은 흐름이라 트리로 바꿀 이유가 없음).
- `WindowSizeMsg` 핸들러에서 테이블/디테일 너비를 `sidebarWidth`(24)만큼 줄여 사이드바와 겹치지 않게 함.

### 컴파트먼트 경로 표시

사이드바의 "Compartments" 리프 밑에, 현재 위치한 컴파트먼트 경로(root → ... → 현재)를 항상 트리로 함께 그림.

- 새 상태 없이 기존 `m.compPath`(breadcrumb에도 쓰던 값)를 그대로 재사용.
- 가장 마지막 항목(현재 컴파트먼트)만 강조 스타일로 구분.

## Instance 목록에 스펙/메트릭 컬럼 추가 (`internal/registry/instance.go`, `instance_metrics.go`)

Instance 테이블에 `OCPU`, `MEM(GB)`(스펙), `CPU%`, `MEM%`(메트릭) 컬럼을 추가.

- **스펙(OCPU/MEM)**: 별도 API 호출 없이 `ListInstances` 응답의 `Instance.ShapeConfig`(`Ocpus`, `MemoryInGBs`)를 그대로 사용.
- **메트릭(CPU%/MEM%)**: OCI Monitoring(`oci_computeagent` 네임스페이스)에서 최근 10분 평균값 1개를 조회.
  - 메트릭은 인스턴스별이 아니라 **컴파트먼트당 2번**의 `SummarizeMetricsData` 호출(CPU 1번, Mem 1번)로 한번에 조회 후 `resourceId`로 매칭 — 인스턴스 N개당 N번 호출하는 방식은 피함.
  - 조회 실패(모니터링 권한 없음, 에이전트 미설치 등)는 조용히 `-`로 표시되고 인스턴스 목록 자체는 정상 표시됨 — 별도 에러 처리 불필요.
  - Monitoring API 호출에는 테넌시/컴파트먼트에 `read metrics` IAM 권한이 필요 — 없으면 CPU%/MEM%는 계속 `-`로만 보임.
- **Storage(부트볼륨 크기)는 스킵** — `Instance` 응답에 없고, 인스턴스마다 별도 API 2콜(boot volume attachment 조회 + boot volume 상세 조회)이 필요해 N+1 비용이 큼. 필요하면 추가 가능.
- `Row.Raw`는 기존 `core.Instance` 대신 `instanceRow{core.Instance, Metrics instanceMetrics}`로 감쌈 — `Columns()`의 타입 단언(`row.Raw.(instanceRow)`)만 이 파일 안에서 바뀌었고, 다른 코드(`bastion.go` 등)는 `Row.Raw`를 참조하지 않아 영향 없음.
- `internal/clients/factory.go`에 `Monitoring(region)` 클라이언트 캐시 추가 (기존 identity/vcn/compute/bastion/lb와 동일 패턴).
