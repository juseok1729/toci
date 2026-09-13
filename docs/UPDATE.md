# toci — 작업 기록

버전(태그)별 변경사항. 배경/이유가 코드만 봐서는 안 드러나는 결정 위주로 기록.

## v0.1.20

### VCN 컬럼: STATE 제거, IP RANGE 추가

- "vcn 컬럼에서 state 제거하고 cidr 다음에 iprange 추가해줘" → STATE 컬럼 제거, CIDR 다음에 Subnet과 동일한 `cidrRange`(사용 가능 호스트 수 포함) 재사용으로 IP RANGE 컬럼 추가.

### 서브트리 모드에서 VCN 리소스맵(`M`)이 VCN만 나오는 버그

- "vcn 서브트리모드(C)상태에서 리소스맵(M)을 키면 vcn만 나와. 서브넷/라우팅테이블/게이트웨이 정보가 안나와" — 서브트리 모드에서는 VCN 행이 현재 보고 있는 컴파트먼트(`m.scope.CompartmentID`, 서브트리 시작 기준)가 아니라 하위의 다른 컴파트먼트에서 왔을 수 있는데, 리소스맵(과 "m" 다이어그램 export)이 서브넷/라우팅테이블/게이트웨이를 조회할 때 항상 `m.scope.CompartmentID`만 필터로 써서, OCI API의 CompartmentId+VcnId AND 필터 조건에 걸려 조용히 0건이 조회됐다.
- `registry.Row`에 `CompartmentID` 필드를 추가해 서브트리 fan-out이 각 행에 실제 소속 컴파트먼트를 채우고, `selectVcnFilter`/`selectDrgFilter`("i"/Enter로 필터 선택 시)가 이 값으로 `m.scope.CompartmentID`를 갱신하도록 수정 — 같은 근본 원인이라 `M`뿐 아니라 `m` 다이어그램 export도 함께 고쳐짐. VCN 테이블에서 직접 `M`을 누르는 단축키 경로도 커서 행의 `CompartmentID`를 우선 사용하도록 반영.

### DRG Attachment: ATTACHED TO를 OCID 대신 이름으로

- "drg attachment에 attached to에 vcn ocid가 있는것같은데 이름으로 보고싶어. 다른 리소스라고 할지라도 ocid가 아닌 이름태그로 볼수있으면 좋겠어" → 연결 대상 종류(VCN/Virtual Circuit/Remote Peering Connection/IPSec Tunnel)별로 각각 다른 Get API로 이름을 조회해 표시. IPSec Tunnel은 터널 자체 DisplayName이 보통 비어있어 상위 IPSec Connection 이름으로 폴백, 조회 실패 시 OCID로 폴백.
- 행마다 추가 API 호출이 하나씩 필요해서 DB System/Instance/Exadata와 동일한 패턴(행별 goroutine fan-out, 각자 자기 인덱스에만 쓰기)으로 병렬화.

### NSG / DRG Route Table 규칙 뷰(`v`)

- "nsg도 v로 rules 테이블뷰 볼수있으면 좋겟어" / "drg route tables도 v로 규칙테이블 볼수있으면 좋겠어" → Security List/Route Table과 달리 NSG·DRG Route Table의 규칙은 오브젝트에 내장돼 있지 않고 별도 리소스(`ListNetworkSecurityGroupSecurityRules`/`ListDrgRouteRules`)라, 기존 동기 방식(`securityRulesView`/`routeRulesView`) 대신 리소스맵처럼 비동기 `tea.Cmd`로 구현(`internal/app/nsg_rules.go`, `drg_route_rules.go`).
- NSG 규칙 포맷/렌더링은 Security List의 `renderSecurityRules`/`securityRuleHeaders`를 그대로 재사용(같은 필드를 담은 다른 SDK 타입이라 매핑만 다름).
- DRG Route Table의 NEXT HOP도 위 "이름으로 보고싶다" 요청과 같은 맥락으로, 해당 DRG의 attachment 목록을 한 번 조회해 OCID→이름으로 매핑(못 찾으면 OCID 폴백), BLACKHOLE 라우트는 "BLACKHOLE"로 표시.
- 둘 다 `v`로 열고 esc/q/v로 닫는 토글, CSV export 모두 기존 규칙 뷰와 동일하게 동작.

## v0.1.19

### VCN 리소스맵 (`M`)

- "feature/resource-map 브랜치 만들어서 [AWS 콘솔 스크린샷]처럼 vcn 리소스맵 구현해줘" — AWS 콘솔의 VCN 리소스맵을 참고해 VCN/Subnet/Route Table/Network Connection을 컬럼으로 나열하고 ASCII 커넥터 선으로 잇는 인앱 시각화를 신규 구현(`internal/app/resource_map.go`, `resource_map_layout.go`). Route Table 규칙을 실제로 조회해서 서브넷→라우트 테이블→게이트웨이 연결을 계산하고, 사용되지 않는 라우트 테이블/게이트웨이는 그려넣지 않음.
- "라우팅테이블을 통해서 어느 서브넷에서 어느 게이트웨이로 가는지에 따라 경로를 하이라이트 해줄수있나?" → j/k로 서브넷을 선택하면 그 서브넷의 라우트 테이블·게이트웨이 경로 전체(박스 테두리+텍스트, 커넥터 선)를 강조색으로 표시(`resourceMapPath`). "하이라이트를 더 밝게하거나 굵게" 요청으로 테두리만이 아니라 텍스트까지 강조색+Bold를 함께 입히도록 수정.
- 박스에 CIDR(서브넷/VCN)·IP(NAT 게이트웨이)를 두 번째 줄로 추가. 색상은 "흰색으로 표시할수있나? 좀 눈에 안띄는것같아서" → 흰색 → "초록색보다는 노란색이 어떨까" → 최종 노란색(`stateTextWarn` 재사용)으로 두 차례 조정.
- "vcn 리소스맵이 페이지가 바뀌어서 표시되는것같은데... 테이블 하단에 플로팅되게해줄수없나?" → 전체 화면을 갈아치우던 `modeDetail` 방식에서, `f` 검색창처럼 테이블 위에 뜨는 하단 오버레이로 전환(`overlayBottom`). 처음엔 화면 높이의 절반으로 시작했다가 "조금만 더 늘려줘" 요청으로 60%(`height*3/5`)로 확대.
- "vcn을 엔터하고 검색창이 뜨고... 너무 번거로워, 원하는 vcn행에 커서를 두고 바로 M으로 띄우고 싶다" → VCN 테이블을 보고 있을 때는 `M`이 커서가 놓인 행을 곧바로 리소스맵으로 빌드(`buildResourceMap(vcnID, vcnName)`가 `m.scope`/영속 필터를 건드리지 않고 그 자리에서만 사용) — 기존에 VCN 필터를 먼저 잡아야(`i`/Enter → 검색창 닫기) M이 동작하던 경로도 그대로 유지.

### 첫 화면을 빈 테이블 + 검색창으로

- "어차피 컴파트먼트를 엔터눌러도 상세정보가 보이게 된거라면, 첫화면은 빈테이블에 검색창이 나오는게 나을것같은데?" → 시작 시 Compartments 목록을 자동 로드하던 것을 제거하고, k9s/taws 스타일로 빈 테이블 위에 리소스 검색창(`f`)을 바로 띄움.

### 리소스 검색 fuzzy 매칭 오탐 수정

- "vcn"으로 검색하면 무관한 "Governance/Compartments"가 매칭되던 버그 — `category + "/" + label`을 하나로 이어붙인 문자열에 fuzzy 매칭을 걸다 보니, 앞뒤로 쪼개진 글자가 카테고리/레이블 경계를 넘어 우연히 이어져도 매칭되던 것. 레이블/카테고리 두 문자열에 각각 별도로 `fuzzy.Find`를 돌려 매칭된 인덱스를 합집합으로 모으고, 원래 목록 순서(카테고리 그룹 유지)로 정렬하도록 수정.

### Subnet TYPE 컬럼 (Public/Private)

- "subnet 에서 컬럼에 Private 인지 Public 인지 알수있는 컬럼 추가해줘" → `ProhibitPublicIpOnVnic` 값으로 판정하는 `TYPE` 컬럼 추가.
- "public 은 초록색, private 은 파란색 계열로 텍스트 색상 적용해줘" → Public은 기존 `stateTextGood`(초록) 재사용, Private은 신규 파란색 스타일(ANSI "12") 추가(`subnet_type_color.go`).

## v0.1.18

### 컴파트먼트를 전역 컨텍스트로 분리 (`c`/`C`/`1`-`9`)

- 사용자 피드백: "컴파트먼트에 모든 리소스가 속해있지만 컴파트먼트가 최상위 필터기 때문에 리소스 스위칭할 때 무조건 컴파트먼트 상속 리소스만 나온다. 근데 수시로 보고 있는 리소스에서 컴파트먼트만 교체할 수 있으면 좋겠다" — 기획서(`NEW_UI.md`)를 같이 작성해서 범위를 정하고 구현. 핵심은 "컴파트먼트는 내비게이션 단계가 아니라 필터"로 재정의하는 것.
- `c` 키로 어떤 리소스 화면에서든 컴파트먼트 트리 피커(`internal/app/compartment_picker.go`)를 열어 현재 리소스 타입은 유지한 채 목록만 재조회. 트리는 앱 시작 시 `ListCompartments(compartmentIdInSubtree=true, accessLevel=ACCESSIBLE)` 한 번으로 전체를 캐시(`compartment_tree.go`)하고, fuzzy 매칭 시 매칭된 노드의 조상은 항상 남겨 경로 컨텍스트를 유지 — 동명 컴파트먼트가 흔한 실사용 테넌시를 고려.
- `C`로 서브트리 모드 토글(F3): 현재 컴파트먼트 + 하위 전체를 fan-out 조회(`subtree.go`). 세마포어(동시 6)로 rate-limit 방어, 결과는 도착 순 스트리밍(먼저 온 컴파트먼트부터 테이블에 반영, 나머지는 "— loading —" placeholder), 401/404는 skip 카운트로만 표시. 테이블 앞엔 `COMPARTMENT` 컬럼이 붙고 헤더엔 `⊕ +N sub` 배지.
- `1`-`9`로 최근 방문 컴파트먼트 즉시 전환(`recent.go`) — `~/.config/toci/recent.yaml`에 프로필별 영속화, 피커에서 `p`로 pin.
- Compartments 리소스 뷰의 역할 변경(F6): Enter가 더 이상 컨텍스트 전환이 아니라 상세보기 — 컨텍스트 전환은 `c` 하나로 통일. 기존에 있던 "compartment 목록이 비면 자동으로 VCN 뷰로 점프"하던 `autoRedirect` 로직은 이 재설계로 전제 자체가 사라져 통째로 제거.
- **버그 1**: 서브트리 모드로 리소스 타입을 바꾸면(예: Instance → Subnet) `switchResource`가 새 컬럼으로 다시 그리기 전에 *이전* 리소스의 fan-out 잔여 데이터를 그대로 렌더링해서 `interface conversion: registry.instanceRow, not core.Subnet` 패닉 발생. `setDisplayRows()` 호출 순서를 fan-out 재시작 이후로 미뤄서 수정, 같은 계열로 서브트리를 끄는 순간에도 placeholder 잔여 행이 남아있으면 동일 패닉이 날 수 있어 `switchCompartment`에도 선제적으로 같은 패치 적용.
- **버그 2**: 서브넷을 VCN으로 그룹핑(`g`)한 상태에서 `c`로 컴파트먼트만 바꾸면 헤더가 VCN 이름 대신 OCID로 표시됨 — `m.vcnNames` 캐시가 컴파트먼트 전환 시 무효화만 되고 재조회 트리거가 없었음(재조회는 `g`를 다시 누를 때만 발생). `switchCompartment`가 그룹핑이 켜져 있으면 재조회 커맨드를 같이 배치하도록 수정.

### Instance 테이블 컬럼 정리

- 사용자 피드백: "유저들이 궁금해하지않는 컬럼이 있다. DOMAIN 빼줘(AD는 1개), 대신 SUBNET, OS 이미지 버전이 필요해" → `DOMAIN(AD/FD)` 제거, `SUBNET`(VNIC의 서브넷을 이름으로 resolve)과 `OS`(이미지의 `OperatingSystem`+`Version`) 컬럼 추가.
- OS 이미지 문구가 길다는 지적에 축약 테이블 추가(`instance_image.go`): `Oracle Linux`→`OL`, `Canonical Ubuntu`→`Ubuntu`, `Windows`→`Win`, 버전 쪽도 `Server`→`Svr`/`Standard`→`Std`/`Datacenter`→`DC`. 커스텀 이미지는 OCI가 OS/버전 필드 둘 다 문자열 `"Custom"`으로 채우는 바람에 "Custom Custom"으로 겹쳐 나오던 것도, 버전이 OS와 같으면 생략하는 일반 규칙으로 수정.
- `USAGE(CPU/MEM %)`(17자)가 실제 값(`23%/45%` 등, 최대 8자 정도)보다 헤더가 훨씬 길어 컬럼 폭을 불필요하게 먹던 것을 `CPU/MEM%`로 축약.
- "disk 정보가 총합인데 부트/블록볼륨으로 나눠달라, 블록볼륨은 여러개 붙을 수 있으니 총합으로, 파일스토리지는 8엑사바이트로 잡히니 제외"라는 요청 → `DISK(GB)`를 `BOOT/BLK(GB)`로 분리(`instance_storage.go` 재작성, boot/block attachment를 별도 map으로 추적). 실제 OCI 블록볼륨 한도(32TB)의 32배인 1PB(`maxSaneVolumeGB`)를 넘는 값은 File Storage 마운트의 오탐으로 보고 합계에서 제외.

### STATE/NODE/EDITION 컬럼 색상이 조용히 사라지는 버그

- 스크린샷 제보: 컬럼이 많아져 폭이 부족해지면 `fitColumns`의 비례 축소가 STATE 컬럼까지 줄여서 "Running"이 "Runni…"로 잘리고, `colorizeState`(state_color.go)가 텍스트 부분일치로 색을 입히는 방식이라 잘린 텍스트는 매칭에 실패해 색이 그냥 사라짐.
- 가로 스크롤바 대신, `fitColumns`가 축소(shrink) 국면에서 `STATE`/`NODE`/`EDITION`(색상 매칭에 쓰이는 컬럼들)만 원래 내용 폭 밑으로 줄지 않게 예외 처리(`shrinkColumns`)하고, 부족한 폭은 나머지 순수 정보성 컬럼이 비례 흡수하도록 수정.

### Subnet: STATE 제거, VCN 그룹 헤더에 CIDR/IP RANGE

- "서브넷은 state가 필요없어" → STATE 컬럼 제거.
- "서브넷에서 g로 vcn 그룹핑하면 vcn 이름이 나오는데... vcn의 cidr와 iprange 도 나왔으면 좋겠어" → `vcnGroupHeader`에 cidr 필드 추가, `treeColumns`가 헤더 행에서 컬럼 제목이 `CIDR`/`IP RANGE`일 때 그 값을 채우도록 확장. IP RANGE 계산은 기존 `registry.cidrRange`를 `CidrRange`로 export해서 재사용(신규 로직 없음).

### 리소스 조회 안정성 (429 재시도, 에러 메시지)

- Route Table 서브트리 조회 중 상태줄이 여러 줄로 깨지는 에러 리포트 → 실제 원인은 OCI Go SDK가 클라이언트/요청에 재시도 정책을 명시하지 않으면 기본값이 `NoRetryPolicy()`(재시도 없음)라는 것. 서브트리 fan-out이 컴파트먼트 수십 개에 동시성 6으로 빠르게 쏘다 보니 Route Table 같은 Networking API의 초당 한도를 건드려 429가 그대로 하드 에러로 노출됨.
- `internal/clients/factory.go`의 클라이언트 생성 공통 지점(`get[T]`)에서 모든 OCI 클라이언트에 SDK 자체의 `common.DefaultRetryPolicy()`(429/5xx/특정 409에 지수 백오프+지터로 최대 8회 재시도)를 설정하도록 수정 — 서브트리뿐 아니라 앱 전체 호출에 적용되는 근본 수정.
- 부수적으로 발견한 문제: `ServiceError.Error()`가 SDK의 여러 줄짜리 트러블슈팅 블록이라, 그걸 그대로 한 줄짜리 상태줄에 넣던 게 화면을 깨고 있었음 → `summarizeSubtreeError`로 상태코드/코드/메시지만 한 줄로 축약.

### Route Table 규칙 뷰(`v`) + 상세보기 토글

- "route table도 security list처럼 v를 누르면 route rules를 테이블뷰로 시각화하는걸 추가해줘" → `internal/app/route_rules.go` 신규, `security_rules.go`와 동일한 패턴(`ltable` 렌더 + CSV export 공유). TARGET 컬럼은 `NetworkEntityId` OCID의 리소스 타입 세그먼트로 추론(Internet/NAT/Service Gateway, DRG, Local Peering GW, Private IP), 모르는 타입은 원본 세그먼트를 그대로 보여줘 조용히 사라지지 않게 함.
- "v 한번누르면 테이블뷰 보이고 v 다시 누르면 되돌아가게 토글되게해줘" → `updateDetail`에서 `v`를 `esc`/`q`와 동급으로 처리해 모든 상세보기(규칙 뷰 포함)를 `v`로도 닫을 수 있게 함.

### 기타

- `shift+↑`/`shift+↓`로 테이블을 절반 페이지씩 스크롤(LazyVim의 ctrl-d/ctrl-u 스타일) — 방향키 한 줄씩만 되던 것에 대한 피드백.
- `f`/`:` 리소스 검색을 평면 목록에서 카테고리 트리(Governance/Compute/Network/Database, OCI 콘솔 좌측 내비와 동일한 분류)로 변경. fuzzy 매칭은 점수순이 아니라 원래 인덱스로 재정렬해서 카테고리가 뒤섞이지 않게 함. "클릭하면 바로 이동해버린다"는 지적으로, 이 트리에서는 클릭이 선택만 하고 Enter로 확정하도록 변경(카테고리 헤더 행이 실수로 클릭되기 쉬운 위치에 껴 있어서).

## v0.1.17

### Exascale 클러스터 노드 트리 (`g` 키) + IP/MEM/OCPU/DISK% 컬럼

- 사용자가 "exascale 리소스테이블도 g를 누르면 클러스터 밑에 트리형태로 노드정보가 나왔으면 좋겠다(서브넷을 VCN으로 그룹핑한 것처럼)"고 요청 → `internal/app/exascale_tree.go` 신규. `vcn_tree.go`의 합성 헤더 방식과 달리, 부모(클러스터)가 실물 리소스라 실제 클러스터 행 바로 뒤에 노드 자식 행을 삽입하는 구조로 구현.
- `registry.ExadbVmClusterRow.Nodes []database.DbNodeSummary` 추가 — 상태 문자열만 뽑던 `fetchDbNodeStates`를 전체 summary를 반환하는 `fetchDbNodes`로 쪼개고 그 위에 얇게 재구성.
- **IP 컬럼 정정**: 처음엔 노드마다 VNIC의 private IP를 썼는데, 사용자가 "g 누르기 전 클러스터 행의 IP는 SCAN IP, g 누른 후 노드별 IP는 host IP여야 한다"고 지적 → 클러스터 행은 `ScanIpIds`, 노드 행은 `DbNodeSummary.HostIpId`를 각각 `GetPrivateIp`로 해석.
- **MEM(GB)/OCPU**: "ecpu 16, memory 44gb인데 vm당 8 ECPU로 만들었다, 메모리도 노드 수로 나눠야 하나?"라는 질문에 실측으로 답함 — `EnabledECpuCount`/`MemorySizeInGBs`는 클러스터 전체 합산값(8×2=16, 44÷2=실제 노드별 22GB와 일치)이라는 걸 확인하고, 클러스터 행엔 총합 `MEM(GB)`을, 노드 트리엔 노드별 `OCPU`/`MEM(GB)`을 추가.
- **DISK%**: "161/300 GB 디스크 사용량을 %로" 요청 → 이 수치가 클러스터 자체가 아니라 연결된 공유 스토리지 볼트(`GetExascaleDbStorageVault`)의 Total/Available이고 161은 Available이지 Used가 아니었다는 걸 실측으로 확인, `(Total-Available)/Total*100`으로 `DISK%` 컬럼 추가.
- `hub-and-spoke`/`vcn-db`의 실제 `ExascaleRAC`(노드 2개)로 매 단계 라이브 검증.

### Exascale DB 노드 SSH 접속 (bastion → jump host → node 체이닝)

- 사용자가 "인스턴스는 Bastion으로 잘 붙는데 Exascale DB 노드에도 접속하고 싶다"며 설계를 제시(노드가 속한 VCN에 bastion이 있으면 그걸로 직접 세션, 없으면 hub VCN의 bastion 사용) → 구현 전 실증 테스트로 **OCI Bastion이 `ExadbVmCluster` OCID를 세션 타겟으로 아예 지원하지 않는다**는 걸 먼저 확인(`CreateSession` → `404 NotAuthorizedOrNotFound`, bastion이 어느 VCN에 있든 마찬가지). 실제 세션 이력을 까보니 지금까지 toci가 만든 세션은 전부 "jump-vm"이라는 평범한 컴퓨트 인스턴스(hub VCN)를 타겟으로 하고 있었고, DB 노드까지는 그 너머로 별도 SSH 홉이 필요했다 — 사용자의 `~/.ssh/config`가 이미 그 구조(`bastion` → `jump` → 노드 IP대역)를 쓰고 있었음.
- 설계를 **Bastion 세션(jump 인스턴스 타겟) → 체이닝된 두 번째 ssh 홉(jump → DB노드)**으로 정정. jump 인스턴스는 bastion이 속한 VCN 안에서 이름에 `jump`/`bastion`이 들어간 것을 찾음(`findJumpInstance`, 검색 범위는 현재 브라우징 중인 컴파트먼트로 한정 — 사용자 확인 결과 이 환경엔 충분).
- 체이닝은 3단 중첩 `-o ProxyCommand="..."` 쿼팅(따옴표 스타일을 번갈아 써야 해서 실수하기 쉬움) 대신, `toci-bastion`/`toci-jump`/`toci-target` 3단 `Host`+`ProxyJump` 스탠자를 담은 임시 ssh config 파일(`buildChainedSSHCommand`)로 생성 — 사람이 직접 쓰는 `~/.ssh/config`와 구조가 동일해 쿼팅 실수 여지가 없다.
- SSH 키는 사용자 결정대로 한 번만 선택해서 jump 세션 생성(필요시)과 DB노드 최종 인증 양쪽에 재사용 — 재사용한 세션이 다른 키를 거부하는 경우는 기존 `embTermExitMsg`의 quickFail 자동 재시도가 그대로 커버(추가 코드 불필요).
- 실제 `HubBastion`에 세션을 새로 만들어 `ExascaleRAC-ssh-key`로 jump-vm 경유 DB 노드까지 SSH 성공(`hostname`/`uptime` 실행 결과 확인) — 검증에 쓴 세션은 즉시 삭제.

## v0.1.16

### Bastion 세션 연결 대기 스피너

- `CreateSession` 폴링(최대 ~90초)이나 `ListSessions`/`GetSession` 조회가 도는 동안 상태표시줄이 정적인 "connecting to bastion..." 텍스트로 멈춰 보이던 문제 → splash 화면과 같은 Braille 스피너(`spinnerFrames`/`spinnerStyle`)를 재사용해 `blinkTickCmd`와 동일한 자체 재스케줄 패턴(`bastionSpinnerTickCmd`, 120ms)으로 애니메이션. 다만 blink와 달리 `Init()`에서 영구히 도는 게 아니라 `bastionPending`이 켜질 때만 시작되고 꺼지면 스스로 멈춤 — 메모리/서버 캐시로 즉시 붙는 경우(대기 자체가 없음)엔 뜨지 않음.

### 재사용한 Bastion 세션이 다른 키를 거부하면 자동으로 새 세션 생성

- 세션 재사용(메모리 캐시든 `findReusableSession`으로 찾은 서버 세션이든)은 그 세션이 **원래 등록됐던 키**로만 인증되는데, 사용자가 이번 접속에서 다른 키를 고르면 SSH가 `exit status 255`로 조용히 실패하던 걸 사용자가 지적 — toci엔 Bastion 세션을 직접 만들고 관리하는 UI가 없으니, 에러로 끝내는 대신 자동으로 새 세션을 만들어야 한다는 피드백.
- `embeddedTerm`에 `startedAt`을 기록하고, 시작한 지 5초(`quickFailWindow`) 안에 죽으면 "애초에 연결이 안 된 것"으로 보는 `quickFail()`을 추가 — 실제로 한참 쓰다가 나중에 끊긴 정상 케이스와 구분하기 위함. `sessionReadyMsg.reused`로 이번 접속이 재사용이었는지 표시해뒀다가, `embTermExitMsg`에서 "재사용 + quickFail" 조합이면 에러 대신 `createSession(..., skipReuse: true)`로 재사용을 건너뛰고 지금 고른 키로 완전히 새 세션을 자동 생성.
- 재시도는 딱 한 번만 — 새로 만든 세션은 `reused = false`이므로, 그것도 quickFail로 죽으면(진짜 키가 잘못됐거나 네트워크 문제) 재귀적으로 또 재시도하지 않고 정상적으로 에러를 보여줌.

## v0.1.15

### SSH 세션을 내장 pty 터미널로 전환

- 기존엔 SSH 실행 시 `HERDR_ENV`/`TMUX` 환경변수로 herdr pane split, tmux new-window, `tea.ExecProcess`(toci 일시 정지 후 복귀) 3-way로 분기했음.
- `internal/app/embedded_terminal.go`(pty + `vt` 터미널 에뮬레이터)로 교체 — 별도 프로그램/멀티플렉서 없이, ssh 세션을 toci 화면 안의 테두리 박스(`modeEmbeddedTerm`)에서 그대로 렌더링.
- `ctrl+\`로 강제 종료, `shift+↑/↓`로 로컬 스크롤백 — 원격 쉘에서 `exit`하면 정상 종료로 테이블로 복귀. 창 크기가 바뀌면(`tea.WindowSizeMsg`) pty 크기도 함께 갱신.

### Exadata VM Cluster (Exascale) 리소스 추가

- 사용자가 "리소스에 exascale도 추가해달라"고 요청 → `database.ExadbVmClusterSummary`/`ListExadbVmClusters`(기존 Exadata Cloud Service의 `CloudVmCluster*`와 나란히 존재하는, Exascale Infrastructure 전용 OCI SDK 타입) 기반으로 `internal/registry/exascale.go` 추가. 기존 `exadata.go`(`CloudVmClusterResource`) 구조를 그대로 미러링.
- OCPU가 아니라 ECPU 기반이라 `EnabledECpuCount`로 ECPU 컬럼을 넣었고, 뒤이어 "LICENSE(edition) 컬럼도 보고 싶다(BYOL로 생성함)"는 요청으로 `LicenseModel`(`BYOL`/`Included`) 컬럼도 추가 — Exascale 요약 타입엔 DB System과 달리 `DatabaseEdition` 필드 자체가 없어서, 실제로 있는 라이선스 모델 필드로 대체했다.
- 다른 DB 계열 리소스(DB System/ADB/Exadata)와 동일하게 VCN 스코프(`vcnScopedResourceKeys`)에 포함시키고, VCN 다이어그램(`m` export) 렌더링에도 cylinder 노드로 추가.

### Bastion 접속 SSH 키 선택 피커 + 세션 재사용

로컬 `~/.ssh`에 여러 키가 있는 실사용 환경(jump-vm 생성 시 다운로드한 `ExascaleRAC-ssh-key-*.key` 등)에서, 기존 `localSSHKeyPair`가 고정된 3개 이름(`id_ed25519`/`id_rsa`/`id_ecdsa`)만 훑어 그중 먼저 찾은 걸 조용히 쓰던 문제를 사용자가 지적 — direct 접속(Bastion 없이 인스턴스 정적 `authorized_keys`로 인증)에서는 실제로 틀린 키를 쓰면 조용히 실패할 수 있었다.

- `listSSHKeyPairs`: `~/.ssh/*.pub` 전체를 훑어서 짝이 되는 개인키가 실제 존재하는 것만 후보로 삼도록 교체(고정 이름 목록 제거). 후보가 1개면 자동 선택, 2개 이상이면 새 피커(`pickerSSHKey`)로 고르게 함 — SSH 모드(bastion/direct) 선택 직후 `resolveSSHKey()`에서 개입.
- **Bastion 경유는 사실 아무 키나 써도 접속된다**는 걸 사용자와의 대화 중 재확인함 — toci가 만드는 건 `CreateManagedSshSessionTargetResourceDetails`(Managed SSH session)이고, 이건 대상 인스턴스의 "Bastion" Oracle Cloud Agent 플러그인이 세션 생성 시 넘긴 공개키를 세션 TTL 동안만 임시로 authorized_keys에 꽂아주는 방식이라, 정적 authorized_keys를 보는 direct 접속과 달리 어떤 로컬 키를 쓰든 상관없다. 그래도 UI 일관성을 위해 피커는 두 경로 모두에 적용.
- **Bastion 세션 재사용**: 접속마다 무조건 새 `CreateSession`을 호출하던 것을 `(bastionID, instanceID, username)` 키로 세션의 ssh 명령 + 만료시각(TTL 1800초)을 `Model.bastionSessions`에 캐싱해 TTL 안에서는 재사용하도록 변경 — Bastion 하나당 동시 세션 수 기본 제한 초과(`LimitExceeded`)와, 매번 세션 ACTIVE 대기(수 초~수십 초)가 들던 비용을 줄임. 재사용 판정엔 2분 여유(`bastionSessionReuseMargin`)를 둬 만료 직전 세션은 새로 만들고, 재사용한 세션이 실제로 죽어 SSH가 에러로 끝나면 그 캐시 엔트리를 즉시 삭제해 같은 실패를 TTL 끝까지 반복하지 않게 함.
- **재사용 캐시가 toci 재시작에서도 살아남게**: `Model.bastionSessions`는 프로세스 메모리일 뿐이라 toci를 껐다 켜면 서버엔 아직 ACTIVE한 세션이 있어도 무조건 새로 만들던 문제를 사용자가 지적 → `createSession`이 새 세션을 만들기 전에 `findReusableSession`으로 해당 Bastion의 `ListSessions(sessionLifecycleState=ACTIVE)`를 훑어 `TargetResourceDetails`(→ `ManagedSshSessionTargetResourceDetails`)의 `TargetResourceId`/`TargetResourceOperatingSystemUserName`이 정확히 일치하고 TTL이 아직 여유있는 세션을 찾으면 `GetSession`으로 전체 세션(SshMetadata 포함)을 가져와 그대로 재사용. 디스크에 캐시를 영속화하는 대신 OCI 자체를 이미 있는 "진짜" 소스로 그때그때 조회하는 쪽을 택함 — 파일 I/O나 직렬화, 여러 toci 프로세스 간 캐시 정합성 문제를 아예 만들지 않음. 인메모리 캐시(`Model.bastionSessions`)는 그대로 두어 같은 프로세스 안에서의 재접속은 API 호출 없이 즉시 재사용되도록 유지(2단 캐시: 메모리 우선, 없으면 서버 조회).

## v0.1.14

### 서브넷 IP RANGE 컬럼 + 사용 가능 호스트 수, AD 컬럼 제거

- 사용자가 "CIDR 옆에 IP RANGE도 보고 싶다"고 요청 → `IP RANGE` 컬럼을 새로 추가, `net.ParseCIDR`로 첫/마지막 주소를 계산해 `10.0.0.0 - 10.0.0.255` 형태로 표시.
- 뒤이어 "사용 가능 호스트 수도 같이" 요청 → 표준 IPv4 관례(전체 주소 - 네트워크/브로드캐스트 2개)로 `(254 usable)`을 같이 붙임. `/31`(RFC 3021 point-to-point, 2개)과 `/32`(호스트 라우트, 1개)는 예외 처리.
- `AD` 컬럼 제거 — OCI SDK 주석 확인 결과 `Subnet.AvailabilityDomain`은 리전 서브넷(Oracle이 권장하는 기본값)이면 항상 null이라 늘 비어 보이던 것. AD-특정 서브넷에서만 값이 차므로, 필요해지면 언제든 되돌릴 수 있음.

### 서브넷 목록 VCN별 트리 그룹핑 (`g` 키)

- 컴파트먼트만 선택하고 VCN 필터 없이 서브넷 전체를 볼 때, 어떤 서브넷이 어느 VCN 소속인지 한눈에 안 보이는 문제 → `g` 키로 VCN별 그룹핑 토글 추가(VCN 필터가 이미 걸려있으면 애초에 단일 VCN이라 키가 동작 안 함).
- **1차 구현(컬럼 방식)**: `VCN` 컬럼을 맨 앞에 추가하고 그 값 기준으로 정렬 — 동작은 했지만, `relayoutTableColumns`가 `refreshTable`과 별도로 컬럼을 계산하다가 그룹핑으로 늘어난 컬럼 개수를 몰라서, 컴파트먼트로 돌아가는 등 relayout이 재호출되는 시점에 `bubbles/table`이 기대하는 컬럼 수와 이미 로드된 행의 셀 개수가 어긋나 `index out of range` 패닉 발생. 컬럼 계산을 `displayColumns()` 공유 헬퍼 하나로 합쳐서 수정.
- **2차 구현(트리 방식)**: AWS 콘솔(Aurora DB identifier 트리형 그룹핑)처럼 부모-자식 트리로 다시 요청받아 `internal/app/vcn_tree.go`에 전면 재작성. 각 VCN마다 합성 헤더 행(`▾ vcn-name`, `vcnGroupHeader` 타입)을 만들어 그 아래 서브넷들을 `├─`/`└─` 커넥터로 들여쓰기. 컬럼 개수는 그대로 두고(1차 패닉의 원인이었던 컬럼 수 불일치가 구조적으로 재발 못 하게) 첫 번째 컬럼의 텍스트만 트리 글자로 장식하는 방식으로 바꿔, 컬럼 추가 없이 안전하게 구현.
- 헤더 행은 실제 리소스가 아니므로 `d`(상세보기)는 무시, `e`(CSV export) 시엔 자동으로 걸러내서 실제 서브넷만 export됨.
- VCN 이름은 처음 켤 때 한 번 조회해서 컴파트먼트 단위로 캐싱(`fetchVcnNames`), 컴파트먼트 이동 시 캐시 무효화. 아직 못 불러왔거나 못 찾은 VCN은 이름 대신 OCID로 표시.
- 이 작업은 `nightly` 브랜치에서 진행 후 `master`로 fast-forward 머지.

## v0.1.13

### Instance / DB System / Exadata 리소스 로딩 병렬화

사용자가 "bumsik 컴파트먼트에서 DB System 목록 불러오는 게 좀 걸린다"고 지적한 게 계기 — 확인해보니 행 하나당 여러 API 콜이 필요한 리소스(DB System, Instance, Exadata)들이 전부 **행 개수만큼 순차로** 호출하고 있었음. 나머지 9개 리소스(Compartment/VCN/Subnet/Security List/NSG/DRG/Route Table/LB/ADB)는 전수 감사해서 행 단위 추가 API 호출이 없는 걸 확인하고 그대로 둠.

- **DB System**: 행마다 `GetDbSystem`(메모리), `ListDbNodes`(노드 상태), `ListDatabases`+`ListDataGuardAssociations`(Data Guard 역할) 최대 4콜이 `for` 루프 안에서 순차 실행되고 있었음. `bumsik`(10개)이면 최악 40콜. → 행별로 goroutine을 fan-out, 각자 **자기 인덱스의 슬라이스 칸(`rows[i]`)에만 쓰기** 때문에 락 없이 안전.
- **Instance**: `fetchInstanceIPs`가 인스턴스마다 `GetVnic`을 순차 호출하고 있었음(단일 NIC이면 인스턴스당 1콜). 여기는 결과를 **map**에 모으는 구조라, DB System과 달리 슬라이스 인덱스 방식을 못 씀 — Go map은 서로 다른 키라도 동시 쓰기가 안전하지 않아서, 각 goroutine의 결과를 **채널로 모아 단일 goroutine에서만 map에 쓰는** fan-out/fan-in 패턴 사용. 추가로 `List()` 최상위에서 metrics/IPs/storage 세 개의 독립적인 조회가 순서대로 실행되던 것도 동시 실행으로 바꿈.
- **Exadata VM Cluster**: DB System과 완전히 같은 패턴(행마다 `fetchDbNodeStates` 순차 호출)이라 동일한 방식(슬라이스 인덱스 fan-out)으로 고침.
- 세 파일 다 `go test -race`로 데이터 레이스 없음 확인. 컴파트먼트당 리소스 개수가 원래 적어서(수십 개 수준) 워커풀/세마포어 같은 동시성 제한은 안 둠 — 전부 한 번에 fan-out.
- **실측**: `bumsik`(DB System 10개, MEM/DISK/NODE/ROLE 다 채운 상태) 약 1.2초, `WYD-POC`(Instance 13개, IP/스토리지/메트릭 다 채운 상태) 약 1.2초.

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
