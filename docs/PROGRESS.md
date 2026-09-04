# toci — 구현 현황 (다음 세션 재개용)

`docs/PLAN_GO.md`의 마일스톤 기준. 이 문서는 계획 대비 실제 구현 상태와, 구현하며 내린 결정/이유를 기록한다. 다음 세션은 이 문서 + `PLAN_GO.md`만 읽고 이어갈 수 있어야 한다.

## 완료

### M1 — 읽기 전용 골격
- `common.CustomProfileConfigProvider` 기반 프로파일 로드 (`--profile` / `OCI_CLI_PROFILE` / 기본 `DEFAULT`)
- `internal/clients.Factory` — 리전별 SDK 클라이언트 캐싱 (`identity`, `core.VirtualNetwork`, `core.Compute`, `loadbalancer`)
- `registry.Resource` 인터페이스 + bubbles/table 뷰, 페이지네이션 헬퍼(`internal/app/paginate.go`가 `OpcNextPage`를 끝까지 순회)

### M2 — 실사용 최소치
- **컴파트먼트 네비게이션**: 계획서는 `CompartmentIdInSubtree`로 트리를 한 번에 받아오는 방식을 제안했지만, **지연(lazy) drill-down 방식으로 단순화**했다. `Enter`로 하위 컴파트먼트 진입, `Esc`로 상위 복귀 — breadcrumb(`internal/app/compartment.go`의 `crumb` 슬라이스)만 유지하고 전체 트리는 만들지 않는다.
  - 이유: 테넌시 레벨 `inspect` 권한이 없어도 항상 동작한다 (`ListCompartments(parentId=현재)`는 일반 권한으로 충분). 계획서가 우려한 "권한 없으면 폴백" 케이스 자체가 발생하지 않는다.
  - 루트 이름은 `identity.GetCompartment(tenancyID)`로 비동기 조회 (실패 시 "root"로 폴백).
- **`f` / `:` 리소스 검색** — `sahilm/fuzzy` 오버레이로 시작해서, 중간에 **왼쪽 사이드바 리소스 트리**로 한 번 교체됐다가, 그 사이드바도 나중에 완전히 제거되고 **다시 이 중앙 오버레이 검색창으로 최종 정착**했다(과정은 아래 "이후 추가/제거된 기능" 참고 — 지금 코드베이스엔 사이드바가 없다). 화면 위쪽 1/3 지점에 뜨는 taws 스타일 박스, 폭은 터미널의 60%(최소 50). `internal/app/picker.go`의 `picker` 타입은 리전/액션/Bastion 피커 3종 + 이 리소스 검색까지 공용으로 쓰인다.
- **`/` 로컬 퍼지 필터** — 이미 불러온 행(`m.rows`)에 대해 이름 기준 클라이언트 사이드 필터링. `Esc`로 되돌리기(편집 시작 시점 값 백업), `Enter`로 확정.
- **리소스 12종**: Compartment, Instance, VCN, Subnet, Route Table, Security List, NSG, DRG, Load Balancer, DB System, Autonomous DB, Exadata VM Cluster (`internal/registry/registry.go`의 `All()`이 표시 순서 그대로).
- **`r` 리전 전환** — `identity.ListRegionSubscriptions`로 구독 리전만 보여줌 (전체 OCI 리전이 아님).
- **`R` 새로고침**.

### M3 (일부) — 인스턴스 액션
- `registry.Actionable` 인터페이스 (`Actions()`, `RunAction()`) — `Resource`와 분리된 별도 인터페이스. 읽기 전용 리소스(VCN, Subnet 등)가 이걸 구현할 필요가 없게 하기 위함.
- `InstanceResource`가 `Actionable` 구현: `start`(`core.InstanceActionActionStart`), `stop`(`core.InstanceActionActionSoftstop` — 강제 STOP이 아니라 graceful softstop을 기본으로 택함).
- **`--write` 플래그**: 기본 `false` = readonly. `a` 키는 `--write` 없이는 어떤 리소스에서도 동작하지 않고 상태줄에 안내만 뜬다.
- **확인 플로우**: `a` → 액션 피커(피커 컴포넌트 재사용) → 선택 시 `modeConfirm`으로 전환, **리소스명을 정확히 타이핑해야만** 실행됨. 이름이 다르면 조용히 취소.
- 실행 후 자동 새로고침, 성공/실패를 상태줄에 표시(실패 시 OCI SDK 에러 메시지 전체 노출 — 409 Conflict 등 실제 API 에러도 패닉 없이 렌더링되는 것까지 실사용 검증 완료).

### M3 (일부) — Bastion 세션 → SSH
- `internal/app/bastion.go`: `listBastions`, `instancePrivateIP`(VNIC 조회 후 private IP), `localSSHKeyPair`(`~/.ssh`에서 `id_ed25519`→`id_rsa`→`id_ecdsa` 순으로 탐색), `createBastionSession`(생성 후 `ACTIVE`까지 최대 90초 폴링), `buildSSHCommand`(응답의 `SshMetadata["command"]`에서 `<privateKey>` 플레이스홀더를 로컬 개인키 경로로 치환).
- Instance 리소스 전용 별도 키 **`s`**로 트리거 (`a` 액션 메뉴가 아니라 독립 키 — start/stop과 달리 단발성 요청-응답이 아니라 "compartment의 Bastion 조회 → (여러 개면 선택) → OS 유저명 입력 → 세션 생성/폴링 → SSH 실행"의 다단계 플로우라 `Actionable.RunAction`의 단순한 fire-and-forget 시그니처에 안 맞았음).
- 대상 compartment에 Bastion이 0개면 상태줄에 안내만 뜨고 끝, 1개면 자동 선택, 2개 이상이면 피커(`pickerBastion`)로 고르게 함.
- OS 유저명은 새 오버레이 `modePrompt`(기본값 `opc`)로 입력받음 — 이미지별로 기본 계정이 다르기 때문에(Oracle Linux는 `opc`, Ubuntu는 `ubuntu` 등) 자동 추정하지 않고 매번 물어봄.
- SSH 실행은 `tea.ExecProcess(exec.Command("sh","-c", sshCmd), ...)`로 위임 — bubbletea가 알아서 `ReleaseTerminal`/`RestoreTerminal`을 해주므로, 계획서가 언급한 수동 `stty sane` + stdin flush는 **불필요했다** (소스로 직접 확인: `exec.go`의 `p.exec()`가 릴리스/복구를 감싸고 있음).
- `--write` 필요 (세션 생성은 실제 클라우드 리소스를 만드는 쓰기 동작으로 취급).

**검증 상태 — 일부만 라이브, 일부는 미검증**: 테넌시의 Bastion 쿼터가 이미 가득 차 있어(`QuotaExceeded`) JCID에 테스트용 Bastion을 새로 만들 수 없었다. 사용자와 상의해 다음까지만 라이브로 확인했다:
- readonly 모드에서 `s` 눌러도 아무 일도 안 일어나는 것 (실제 확인됨)
- JCID처럼 Bastion이 없는 컴파트먼트에서 `s` → "no bastion found in this compartment"까지 실제 `ListBastions` 호출로 확인됨 (패닉 없음)

**아직 라이브로 못 본 것** (코드 리뷰만 완료, 실제 세션으로 검증 안 됨):
- `CreateSession` 요청 필드가 실제로 맞는지 (`TargetResourceOperatingSystemUserName`, `TargetResourcePrivateIpAddress`, `TargetResourcePort` 조합)
- 폴링이 실제로 `ACTIVE`를 잡아내는지
- **`SshMetadata`의 키가 정말 `"command"`이고, 값 안의 플레이스홀더가 정말 `"<privateKey>"` 문자열인지** — OCI CLI의 `session get-connection-string` 동작 기억에 근거해 짜긴 했지만 실제 API 응답으로 검증한 적은 없다. 다음 세션에서 Bastion을 하나라도 쓸 수 있게 되면 **제일 먼저 이 부분부터** 확인할 것 (`fmt.Printf("%#v\n", session.SshMetadata)` 같은 걸로 실제 키/값을 찍어보는 게 가장 빠름).
- 실제 SSH 접속 성공 여부, `tea.ExecProcess` 핸드오프 후 화면 복구가 매끄러운지

재개 옵션: (1) 쿼터 늘려달라고 요청, (2) 안 쓰는 기존 Bastion 하나 정리하고 재시도, (3) 다른 사람의 기존 Bastion을 동의 받고 빌려서 1회 검증 후 정리 — 이 세 가지를 사용자에게 다시 물어봤었다 (해당 세션에서는 (1) 코드만 구현 옵션을 선택함).

## 계획에 없던, 추가로 구현한 기능

계획서(`PLAN_GO.md`)의 마일스톤에는 없지만 이후 세션에서 사용자 요청으로 추가한 것들. 자세한 배경은 `docs/UPDATE.md` 참고.

### Instance 목록 스펙/메트릭 컬럼 (`internal/registry/instance.go`, `instance_metrics.go`)
- Instance 테이블에 `OCPU`, `MEM(GB)`(스펙), `CPU%`, `MEM%`(메트릭) 컬럼을 추가했다.
- **스펙**: 별도 API 호출 없이 `ListInstances` 응답의 `Instance.ShapeConfig`(`Ocpus`, `MemoryInGBs`)를 그대로 사용.
- **메트릭**: OCI Monitoring(`oci_computeagent` 네임스페이스)에서 최근 10분 평균값을 조회. 인스턴스별이 아니라 **컴파트먼트당 2번**의 `SummarizeMetricsData` 호출(CPU 1번, Mem 1번)로 한번에 조회 후 응답의 `resourceId` 디멘션으로 매칭한다 — 인스턴스 N개당 N번 호출하는 방식은 피함.
  - 조회 실패(모니터링 권한 없음, 컴퓨트 에이전트 미설치 등)는 조용히 `-`로 표시되고 인스턴스 목록 자체는 정상 표시된다 — 별도 에러 처리 없음.
  - 테넌시/컴파트먼트에 `read metrics` IAM 권한이 없으면 CPU%/MEM%는 계속 `-`로만 보인다.
- **Storage(부트볼륨 크기)는 스킵** — `Instance` 응답에 없고, 인스턴스마다 별도 API 2콜(boot volume attachment 조회 + boot volume 상세 조회)이 필요해 N+1 비용이 크다. 필요해지면 추가 가능.
- `Row.Raw`는 기존 `core.Instance` 대신 `instanceRow{core.Instance, Metrics instanceMetrics}`로 감쌌다 — `Columns()`의 타입 단언만 이 파일 안에서 바뀌었고, `Row.Raw`를 참조하는 다른 코드(`bastion.go` 등)는 없어서 영향 없음.
- `internal/clients/factory.go`에 `Monitoring(region)` 클라이언트 캐시를 추가 (기존 identity/vcn/compute/bastion/lb와 동일한 캐싱 패턴).

### 사이드바 — 도입했다가 완전히 제거됨 (지금은 존재하지 않음)

한때 왼쪽에 상시 표시되는 리소스 트리(`internal/app/sidebar.go`)가 있었고, 카테고리 재편/폭 계산 버그/Compartments 리셋 등 여러 라운드의 수정을 거쳤다. 하지만 `f`/`:` 중앙 검색창이 생기고 나니 상시 표시 패널이 중복 기능이 되어 **사용자 요청으로 파일째 삭제**했다 — `sidebar.go` 자체가 없고, `resourceCategories`/`isVcnDependent()`/`sidebarAbsFloor` 같은 이름도 코드베이스에 없다. 지금 컴파트먼트 진입/이탈 로직은 아래처럼 정리되어 있다:
- **Compartments 리셋**: 검색창에서 "Compartments"를 고르면 항상 테넌시 루트로 돌아가 최상위부터 다시 드릴다운 (`switchToRootCompartments`) — 안 그러면 리프 컴파트먼트에 있을 때 재선택해도 빈 화면만 반복됐음.
- **컴파트먼트 진입 시 자동 리다이렉트**: 하위 컴파트먼트가 없는 리프에 진입하면 자동으로 **VCN** 목록으로 전환 (`m.autoRedirect`) — 빈 Compartments 화면 대신 뭔가 보여주기 위함. 검색창으로 수동으로 Compartments를 다시 고르는 경우엔 이 자동전환이 발동하지 않도록 별도 플래그로 구분.
- VCN 행에서 **`i` 또는 `Enter`**를 누르면 그 VCN으로 필터가 걸리고 — 사이드바가 있던 시절엔 거기로 포커스가 넘어갔지만, 지금은 **바로 `f` 리소스 검색창이 뜬다** (사용자 피드백: "바로 검색창이 띄워지는게 더 직관적").

### VCN 스코프 필터링 일반화 + DB 리소스 3종 추가

- VCN 목록에서 **`i`(또는 `Enter`)** → 그 VCN을 필터로 지정(`selectVcnFilter`)하고 `f` 리소스 검색창이 뜬다. 이후 VCN-scoped 리소스들 사이를 옮겨다녀도(Subnet↔Instance 등) 필터가 유지되고, Compartments/DRG처럼 스코프 밖으로 나가면 자동 해제.
- **DB Systems**(`db-system`), **Autonomous DBs**(`adb`), **Exadata VM Clusters**(`exadata`, 실제로는 `CloudVmCluster` — 물리 인프라 계층인 `CloudExadataInfrastructure`는 SubnetId가 없어 VCN 종속이 아니라서 제외) 3종 추가. 전부 자체 `SubnetId` 필드가 있어서 Instance처럼 VNIC 조인이 필요 없음.
- Instance의 VCN 필터링에 쓰던 `instanceIDsInVcn`을 리팩터링해서 `registry.InstanceSubnetIDs`(인스턴스ID→서브넷ID 전체 맵)를 export — 나중에 Mermaid 다이어그램에서 재사용.

### Instance 컬럼 재정렬 + STATE 배경색 (제일 오래 걸린 버그 사냥)

컬럼 순서를 `NAME, IP(PUB/PRI), SHAPE, OCPU, MEM(GB), USAGE(CPU/MEM %), DOMAIN(AD/FD), STATE`로 정리하고, STATE의 RUNNING/STOPPED에 배경색(초록/빨강, 커서 있는 행은 더 어둡게)을 입히는 과정에서 구조적 버그를 세 번 연달아 만났다:

1. **셀 값에 ANSI를 직접 심는 방식은 안 된다** — bubbles 테이블이 `go-runewidth`로 셀을 자르는데, 이 라이브러리가 ANSI 이스케이프 바이트를 전부 "보이는 글자"로 세서 색칠된 값이 중간에 잘리고 리셋 코드까지 날아가 색이 뒤 칸으로 번짐.
2. **컬럼 폭을 넉넉히 줘서 안 잘리게 해도, 리셋을 `\x1b[0m`(전체 리셋)으로 하면 선택된(커서) 행의 하이라이트가 STATE 이후 칸부터 끊김** — 리셋이 "내가 선택 행 스타일 안에 있다"는 걸 모르기 때문.
3. **최종 해법**: 색칠을 셀 값이 아니라 **테이블이 다 그려진 뒤 최종 문자열에 후처리**로 입힘 (`internal/app/state_color.go`). `charmbracelet/x/ansi`(lipgloss가 이미 의존 중이던 걸 직접 의존성으로 승격)의 `ansi.Cut`으로 STATE 컬럼 구간만 잘라 배지를 끼워넣는데, 이 함수는 자르는 지점 이전에 열려있던 스타일(선택 행 하이라이트)을 뒷부분에 그대로 이어붙여줘서 문제가 해결됨.
   - **컬럼 폭 자동 축소와의 상호작용 버그**: 이후 모든 컬럼에 fit-to-content(`fitColumns`, 창 크기 비례 축소 포함)를 적용하면서, `colorizeInstanceState`가 STATE 위치를 계산할 때 여전히 `registry.Column`의 **선언된(고정) 폭**을 쓰고 있어서 실제 렌더 폭과 어긋나 색이 안 나오는 회귀가 생겼다 — `m.current().Columns()` 대신 `m.table.Columns()`(bubbles가 실제로 렌더링에 쓴 폭)를 쓰도록 고쳐서 해결.
   - 검은 글씨(`Color("0")`)에 `Bold(true)`를 같이 쓰면 터미널이 "bold=밝게"로 해석해서 회색으로 보이는 것도 발견 — 256색 팔레트의 순수 검정(`Color("16")`)으로 바꿔서 해결.
- **이 시점의 배색(배경 배지, fg=검정/bg=초록·빨강)은 이후 완전히 대체됐다** — 지금은 배경 배지가 아니라 **텍스트 색**이고, RUNNING/STOPPED 두 값만이 아니라 모든 리소스의 STATE에 적용되는 3단계(Good/Bad/Warn) 체계다. `ansi.Cut` 후처리 기법 자체(위 1~3번)는 지금도 그대로 쓰인다 — 자세한 최종 형태는 아래 "STATE/NODE/EDITION 색상 체계 최종 형태" 절 참고.

### 테이블 높이가 필터 입력마다 계속 줄어들던 버그

`refreshTable()`이 테이블을 새로 만들 때 `m.table.Height()`를 읽어 그대로 `newTable()`에 넘겼는데, bubbles의 `Height()`는 이미 헤더 줄만큼 뺀 값을 돌려주고 `newTable()`이 쓰는 `WithHeight()`는 또 헤더 줄만큼 빼는 구조라 — 값을 한 바퀴 돌릴 때마다 높이가 1줄씩 깎였다. 필터는 키 누를 때마다 `refreshTable()`을 호출하니 타이핑할수록 빠르게 줄어들어 눈에 띈 것. "헤더 빼기 전" 원래 높이를 `m.tableHeight`에 별도로 저장해두고, 테이블을 새로 만들 때 이 값을 쓰도록 고침.

### 상태줄이 터미널 폭을 넘어가 화면이 밀리던 버그

상태줄에 필터 문자열 + 여러 힌트가 다 붙다 보니 터미널 폭을 넘는 경우가 있었는데, 그러면 **터미널이 자체적으로 줄바꿈**해버려서 bubbletea의 줄 수 계산과 어긋나 화면이 프레임마다 한 줄씩 밀렸다. 상태줄과 필터 입력줄에 `lipgloss.MaxWidth(mainContentWidth)`를 적용해 폭을 넘는 부분은 줄바꿈 대신 잘리게 고침.

### 헤더를 taws 스타일 블록으로 재구성

```
Profile:  ETEVERS
Region:   ap-seoul-1
Resource: Instances    wydsofficial/WYD-SOLUTION › wyd-solution-vcn
```
`Profile`은 이전엔 어디서도 안 쓰이던 값이라 `app.New()`에 파라미터로 추가해서 `main.go`에서 넘김. 헤더가 1줄(+빈줄)에서 3줄(+빈줄)로 늘어난 만큼 테이블/디테일/사이드바 높이 계산에 +2 반영.

### 스페이스바 which-key 팝업 (`internal/app/help.go`)

LazyVim처럼 스페이스바로 우측하단에 단축키 목록 팝업을 띄움. 항상 떠 있던 회색 가로 힌트 줄(`renderStatusLine`)은 없애고 `N items · space: shortcuts`만 남김.
- **진짜 떠 있는 오버레이**: 별도 레이어를 그리는 게 아니라, STATE 색칠에 썼던 것과 같은 `ansi.Cut` 방식으로 렌더링된 화면의 우측하단 사각형 영역을 실제로 잘라내고 박스를 끼워 넣는 방식(`overlayBottomRight`).
- 스페이스로 토글, 팝업이 떠 있을 때 다른 키를 누르면 그 키의 동작이 그대로 실행되면서 팝업도 같이 닫힘(LazyVim처럼). Esc는 팝업만 닫고 "위로 가기" 동작은 발동 안 시킴(한 번 더 눌러야 함).

### CSV export (`e`) + Mermaid 다이어그램 export (`m`)

- **`e`**: 현재 화면에 보이는(필터 적용된) 행을 리소스의 컬럼 정의 그대로 CSV로 저장 (`internal/app/export.go`). UTF-8 BOM을 붙여서 엑셀(Windows)에서 한글 안 깨지고 바로 열림. Security List에서 `v`로 규칙 테이블을 보고 있을 때도 그 규칙들을 같은 방식으로 export 가능(`m.detailExport`) — 렌더링과 export가 같은 데이터(`securityRuleRecords`)를 써서 화면과 파일이 항상 일치.
- **`m`**(VCN 필터가 걸려있을 때만): 그 VCN의 서브넷별로 Instance/DB System/ADB/Exadata를 그룹핑하고, 그 VCN에 붙어있는 DRG까지 포함해서 Mermaid **`graph TD` + 중첩 `subgraph`**(플로우차트 문법) 텍스트를 `.mmd` 파일로 저장 (`internal/app/diagram.go`). 여러 리소스를 새로 fetch해야 해서 비동기(`tea.Cmd`)로 처리.
  - DRG는 VCN 자체에 붙는 거라(서브넷 소속이 아님) `vcn` subgraph 밖에 별도 노드로 두고 `drgN --> vcn`으로 연결 (flowchart에서 subgraph 자신의 id를 엣지 끝점으로 쓰면 그 경계에 붙는다). `ListDrgAttachments(vcnId=...)`로 이 VCN에 붙은 DRG만 걸러냄.
  - 리소스는 아이콘 대신 노드 모양으로 구분: Instance는 사각형(`[...]`), DB 3종(DB System/ADB/Exadata)은 실린더(`[(...)]`).
  - **`architecture-beta`(mermaid.js의 클라우드 아키텍처 전용 문법)로 3차례 시도했다가 최종적으로 포기하고 이 버전(`graph TD`+`subgraph`)으로 되돌렸다.** 순서: (1) 라벨 따옴표 문제로 Notion 파싱 에러 → 따옴표 처리(`["wyd-logistics-drg"]`)로 해결. (2) 형제 그룹(서브넷) 간 엣지가 없으면 레이아웃이 다 겹쳐서 별 모양(star)으로 `vcnhub`에 전부 연결 — 근데 4방향(R/L/T/B)을 `i%4`로 돌려쓰다 보니 서브넷 5개 이상에서 포트가 재사용되며 다시 겹침. (3) 별 대신 `vcnhub → drg0 → sub0 → sub1 → ...`처럼 매 홉마다 새 노드 쌍을 쓰는 체인으로 바꿔 포트 재사용 자체를 구조적으로 없앴는데도, Notion 실사용 확인에서 **서브넷 크기가 서로 크게 다른 경우(6개 서비스짜리 vs 빈 서브넷) 여전히 다르게 겹치는 걸 확인**했다. 세 번 다 Notion에서 직접 확인된 실패라, `architecture-beta`의 auto-layout 자체가(아직 베타 단계) 비대칭 그룹 크기를 못 다루는 걸로 결론 내리고, `mermaid.parse()`로 문법만 맞다고 레이아웃까지 보장되는 게 아니란 걸 확인한 뒤 — 성숙하고(dagre 기반) 수동 anti-overlap 토폴로지가 필요 없는 `graph TD`+`subgraph`로 최종 복귀(사용자 승인).
  - 문법 검증은 실제 mermaid.js 파서로 했다(Node+JSDOM `mermaid.parse()`). 완전한 SVG 렌더(`mmdc`)는 샌드박스에 Chromium 의존 시스템 라이브러리(`libasound.so.2`)가 없어서(sudo 필요) 못 봤음 — Notion 실사용이 실질적인 렌더링 검증 수단.
- xlsx는 stdlib에 없어서 새 의존성이 필요해 스킵 — BOM CSV로 엑셀 호환 문제는 이미 해결되니 필요해지면 그때 추가.

### Security List 규칙 테이블 뷰 (`v`)

Security List에서 `v`를 누르면 ingress/egress 규칙을 YAML 대신 `lipgloss/table`(lipgloss에 이미 포함된 서브패키지, 새 의존성 아님)로 만든 표로 보여줌. Protocol 번호(`6`→TCP 등)를 이름으로, 포트도 읽기 쉽게 변환. 기존 Enter(YAML 상세)는 그대로 두고 별도 키로 추가.

### 상태 메시지 색상

`m.statusMsg`가 "exported"/"diagram written"으로 시작하면 초록, "failed"/"error"를 포함하면 빨강, 나머지는 기존 회색 — `renderStatusMsg()` 하나로 상태줄과 모드-디테일 화면(Security List 규칙 뷰) 양쪽에서 재사용.

### 시작 스플래시 화면 (`internal/app/splash.go`)

taws(Rust AWS TUI) 스타일 참고해서 만든 시작 화면 — ASCII 로고, 가짜(사실상) 진행 바(실제 신호는 초기 로드 1건뿐이라 단계별로 점프하게 해서 "살아있어 보이게" 함), 브레일 스피너, 단계 바뀔 때마다 재추첨되는 랜덤 문구. 스플래시 전용 스타일(`splashLogoStyle` 등)을 메인 UI 팔레트와 분리해서, 나중에 메인 UI 테마를 바꿔도 스플래시/우측상단 코너 로고는 안 바뀌게 함(둘 다 OCI 브랜드 레드 `196` 고정).

### STATE/NODE/EDITION 색상 체계 최종 형태

RUNNING/STOPPED 배경 배지(위 "제일 오래 걸린 버그 사냥" 절)에서 출발해 여러 차례 리테마를 거쳐 지금 형태로 정착했다:

- **OCI 그린 팔레트**로 전체 UI 크로스 리테마 — OCI 콘솔 사이드바 스크린샷에서 픽셀 색을 실측 추출(`ociAccent`/`ociBorder`/`ociSubtle`/`ociSelBg`/`ociHighlt`, `internal/app/model.go`). 스플래시/코너 로고와 성공·실패 같은 의미색은 이 팔레트에서 의도적으로 제외.
- **STATE**: 배경 배지 → **텍스트 색**으로 전환(`internal/app/state_color.go`). 선택 행에서 색만 다르고 배경은 그대로 두면 `style.Render()`의 리셋이 구멍을 내서, `*Selected` 변형에 `Background(ociSelBg)`를 별도로 얹는 패턴이 여기서도 반복됨.
- **컬럼 재배치**: STATE를 각 리소스의 NAME 바로 다음 컬럼으로 통일(원래 대부분 맨 끝).
- **표시 포맷**: `stateLabel()`(제네릭 `[T ~string]`)이 SDK의 전부-대문자 enum(`NEEDS_ATTENTION`)을 `Needs Attention`처럼 Title Case로 변환 — 12종 리소스의 서로 다른 `LifecycleState` enum 타입에 전부 재사용됨.
- **3단계 색상 등급**(`colorizeState`, 컬럼 제목을 인자로 받아 STATE/NODE 양쪽에 재사용): Good(초록: Running/Active/Available/Standby) / Bad(빨강: Stopped/Failed/Inaccessible/Unavailable) / Warn(노랑: Needs Attention). 판정 순서는 **Warn→Bad→Good 고정** — ADB의 `Available Needs Attention`처럼 여러 등급 단어가 같이 들어있는 값이 있어서, 순서를 안 지키면 "Available"이 먼저 걸려 Good으로 오판정됨. 리소스별 실제 상태값 전수 목록과 등급 매핑은 `docs/COLOR_SYSTEM.md` 참고.
- **NODE 컬럼**(아래 "DB System/Exadata 강화" 참고)의 여러 값(`Available/Stopped`)은 `/`로 쪼개서 **부분별로 독립 색칠** — 통짜 한 색이 아니라 초록/빨강이 따로 보여서 어느 노드가 문제인지 바로 보임. 선택 행에서는 `/` 구분자도 `selStyle`로 칠해야 그 자리만 하이라이트가 끊기는 걸 막을 수 있었음.
- **EDITION**(DB System 전용)은 상태/헬스 신호가 아니라 티어 구분이라 `colorizeState`에 안 얹고 `internal/app/edition_color.go`로 완전히 분리, 정확히 일치(`==`) 매칭(`EE`가 `EE-HP`/`EE-EP`/`EE-DEV`의 접두어라 `Contains`면 오작동). 일반 행은 비비드 색(SE2 시안/EE 블루/EE-HP 퍼플/EE-EP 핫핑크/EE-DEV 그린), **선택 행에서는 파스텔(연한) 톤**으로 별도 지정 — 처음엔 검정 배지, 그다음 어두운 톤을 시도했다가 둘 다 사용자 피드백으로 롤백하고 파스텔로 정착(밝기 차이로 대비를 주는 방식이 `ociSelBg`의 채도와 안 부딪힘).

### 최근 생성/수정된 행 깜빡임 (`nightly` 브랜치에서 개발 후 병합)

3일 이내 생성된 리소스의 행이 깜빡이는 기능(`blinkRecentRows`). 원래는 OCI Audit Service까지 연동해서 "수정"도 감지하려 했으나, 실측해보니 감사 로그 양이 많은 테넌시에서 `--all` 페이지네이션이 2분 넘게 안 끝나는 성능 문제와 `POST=수정`이라는 휴리스틱의 오탐(모니터링 조회성 API도 POST를 씀) 둘 다 발견되어 **범위를 생성일자(TimeCreated)만 보는 것으로 축소**했다. `b` 키로 기능 자체를 껐다 켰다 가능(기본 켜짐).

### DB System / Exadata VM Cluster 관측성 강화

- **NODE 컬럼**: DbSystem/CloudVmCluster의 STATE(`Available`)는 리소스 자체 상태일 뿐, 그 안의 실제 DB Node는 별도 LifecycleState를 갖는다 — 노드만 정지시켜도 리소스는 계속 Available로 보임(실제로 한 테스트 컴파트먼트의 DB System 10개 전부에서 이 패턴 확인). `ListDbNodes`로 조회하는데, 이 API가 `DbSystemId`나 `VmClusterId` 중 하나가 반드시 있어야 하는 제약이 SDK 구조체 태그(둘 다 `mandatory:false`)엔 안 드러나 있어서 실제로 호출해보고서야 발견(`internal/registry/db_node.go`).
- **ROLE 컬럼**(DB System 전용): 2노드 이상이면 무조건 RAC(OCI에서 노드 여러 개인 VM DB System은 RAC 외엔 존재 불가), 아니면 Data Guard 역할(Primary/Standby, `ListDatabases`→`ListDataGuardAssociations`)을 표시. Standby가 다른 리전에 있으면 그 리전 목록엔 아예 안 뜨는데(목록 조회 자체가 리전 스코프), `PeerDbSystemId`(OCID 안에 리전이 그대로 박혀있음)를 파싱해서 **추가 API 호출 없이** `Primary→ap-tokyo-1`처럼 힌트를 붙임(`internal/registry/db_role.go`).
- **MEM(GB)**: `ListDbSystems`엔 항상 nil이고 `GetDbSystem`에서만 채워짐(실측 확인) — DB System은 컴파트먼트당 적어서 행마다 Get 콜 하나 추가하는 비용은 감수.
- **DISK(GB)**: `DataStorageSizeInGBs`(DATA)+`RecoStorageSizeInGB`(RECO) 합산, 이건 `ListDbSystems`에 이미 있어서 추가 호출 불필요.
- **VERSION/EDITION**: `Version`도 List에 바로 있음. EDITION은 원본 enum이 너무 길어서(`ENTERPRISE_EDITION_EXTREME_PERFORMANCE`) Oracle 라이선싱 문서의 실제 축약어(`SE2`/`EE`/`EE-HP`/`EE-EP`/`EE-DEV`)로 변환(`dbEditionAbbrev`).
- **컬럼 폭 상한 버그**: `fitColumnWidth`의 `Width`는 힌트가 아니라 **하드 상한**이라, 처음엔 짧은 값만 보고 좁게 잡아뒀던 ROLE/NODE 컬럼이 긴 값(크로스리전 힌트, 다수 노드)에서 잘릴 뻔했다 — 실제 최악값 기준으로 재계산(ROLE/DB System NODE: 25, Exadata NODE: 103)하고 회귀 테스트로 고정.

### Instance / DB System / Exadata 리소스 로딩 병렬화

행 하나당 API 콜이 여러 개 필요한 세 리소스가 전부 **행 개수만큼 순차 호출**하고 있던 걸 발견 (컴파트먼트 하나에 DB System 10개면 최악 40콜, 몇 초씩 걸림). 행별로 goroutine을 fan-out해서 고침:
- DB System/Exadata는 결과를 **인덱스 슬라이스**(`rows[i]`)에 쓰기 때문에 락이 필요 없음.
- Instance의 `fetchInstanceIPs`는 결과를 **map**에 모으는 구조라 슬라이스 방식을 못 씀 — Go map은 서로 다른 키라도 동시 쓰기가 안전하지 않아서, 각 goroutine 결과를 채널로 모아 **단일 goroutine에서만 map에 쓰는** fan-out/fan-in 패턴을 씀. Instance List() 최상위의 metrics/IPs/storage 세 독립 조회도 순차 → 동시 실행으로 바꿈.
- `go test -race`로 데이터 레이스 없음 확인. 나머지 9개 리소스는 감사해서 행 단위 추가 API 호출이 없는 걸 확인하고 그대로 둠.
- 실측: DB System 10개/Instance 13개 컴파트먼트 각각 약 1.2초(전부 enrichment 포함).

### Instance "Enable Monitoring" 액션 (코드만 완료, 라이브 미검증)

Oracle Cloud Agent의 "Compute Instance Monitoring" 플러그인을 `--write` 모드에서 켤 수 있는 액션 추가 — `UpdateInstance`에 `AgentConfig{IsMonitoringDisabled: false, PluginsConfig: [{Name: "Compute Instance Monitoring", DesiredState: ENABLED}]}`를 보낸다. 다른 플러그인(Bastion 등)은 `PluginsConfig`에 안 넣어서 안 건드림 — OCI가 플러그인을 개별 업데이트하는 방식이라 명시 안 한 플러그인은 현재 상태 유지. **실제 인스턴스 설정을 바꾸는 동작이라 사용자 동의 없이 라이브 테스트는 안 했다** — 기존 start/stop과 완전히 같은 구조(`Actionable` 인터페이스, confirm-by-typing-name)라 빌드/테스트만 확인. 다음 세션에서 `--write`로 직접 켜보고 실제로 반영되는지 확인 필요.

## 계획에 없던, 구현하며 발견한 이슈

**bubbles table/viewport의 내부 상태 버그**: 기존 `table.Model`에 `SetRows()`로 더 적은/다른 행을 밀어넣으면, 이전 커서·스크롤 오프셋(YOffset)이 새 행 수와 안 맞아 `viewport.visibleLines()`에서 `slice bounds out of range` 패닉이 난다 (bubbles v1.0.0 기준, `clamp()`가 `low > high`일 때 값을 스왑하는 구현 때문에 top > bottom인 슬라이스가 만들어짐).

**대응**: 리소스 전환·필터 적용·새로고침 등 행 집합이 바뀌는 모든 지점에서 기존 테이블을 **mutate하지 않고 `newTable()`로 새로 생성**한다 (`internal/app/model.go`의 `refreshTable()`). 이 패턴을 벗어나면(즉 기존 `table.Model`에 직접 `SetRows`/`SetCursor`를 호출하면) 같은 클래스의 패닉이 재발할 수 있으니, 새 코드에서도 이 규칙을 지킬 것.

## 아직 안 한 것

### M3 나머지
- Bastion 세션/SSH 코드는 구현됐지만 **실제 세션으로 검증되지 않았다** — 자세한 내용과 재개 방법은 위 "M3 (일부) — Bastion 세션 → SSH" 절 참고. 가장 먼저 할 일은 Bastion 하나를 확보해서 `SshMetadata`의 실제 키/값 포맷부터 확인하는 것.
- 세션 정리(session delete)는 구현 안 함 — 세션은 TTL(현재 1800초 고정)로 자동 만료되므로 없어도 동작엔 지장 없지만, 명시적으로 끝내고 싶을 때를 위해 `d`(delete session) 같은 키를 나중에 추가할 수 있음.

### 그 외 미검증/미구현

- **"Enable Monitoring" 액션(Instance)**: 코드는 완성됐지만 **`--write`로 실제 인스턴스에 대고 실행해본 적이 없다** — 다음 세션에서 가장 먼저 확인할 것 (위 "Instance Enable Monitoring 액션" 절 참고).
- **다른 Oracle Cloud Agent 플러그인(예: Bastion) 켜기/끄기**: 메커니즘(`UpdateInstance`+`AgentConfig.PluginsConfig`)은 Enable Monitoring과 완전히 동일해서 기술적으로는 바로 확장 가능하지만, 아직 요청받지 않아 구현 안 함.

### M4 — 배포
- GoReleaser 크로스 컴파일 + Homebrew tap
- Docker 이미지 (선택)

## 파일 지도

```
cmd/toci/main.go              cobra 진입점, --profile/--region/--write
internal/clients/factory.go   리전별 SDK 클라이언트 캐시 (identity/vcn/compute/bastion/lb/monitoring/database/blockstorage)
internal/registry/
  resource.go                 Resource, Actionable, Column, Row, Scope 인터페이스/타입 (Scope.VcnID, Row.TimeCreated 포함) + stateLabel()
  compartment.go ~ lb.go      리소스 12종 구현체 (compartment/instance/vcn/subnet/route_table/security_list/nsg/drg/lb/db_system/adb/exadata)
  db_system.go / adb.go / exadata.go   DB Systems / Autonomous DBs / Exadata VM Clusters (전부 VCN-scoped)
  db_node.go                  DbSystem/CloudVmCluster 공용 — ListDbNodes로 노드별 상태 조회 (행별 goroutine 병렬)
  db_role.go                  DB System 전용 — RAC/Data Guard Primary·Standby 역할 조회 + 크로스리전 힌트
  instance_metrics.go         Instance CPU%/MEM% 조회 (Monitoring, 컴파트먼트당 2콜)
  instance_ip.go              Instance Public/Private IP 조회 (VNIC 조인, 인스턴스별 goroutine 병렬 + 채널로 map 수집)
  instance_storage.go         Instance 부트+블록 볼륨 합산 (AD당 4콜, 인스턴스 수와 무관)
  instance_vcn_filter.go      VCN 필터용 서브넷/인스턴스 조인 헬퍼 + InstanceSubnetIDs(export, 다이어그램용)
  registry.go                 All() — UI에 노출되는 리소스 순서
internal/app/
  model.go                    bubbletea Model — 상태머신 본체 (Update/View), 리소스 검색(f/:)·컬럼 폭 자동조절(fitColumns)도 여기
  state_color.go              STATE/NODE 텍스트 색 후처리 (ansi.Cut 기반, 3단계 Good/Bad/Warn, "/" 단위 개별 색칠) — 모든 리소스 공용
  edition_color.go            DB System EDITION 축약어 전용 색상 (state_color.go와 별도 팔레트)
  splash.go                   시작 스플래시 화면 (ASCII 로고, 진행 바, 스피너)
  help.go                     스페이스바 which-key 팝업 (우측하단 오버레이)
  export.go                   CSV export 공통 로직 (UTF-8 BOM)
  diagram.go                  VCN 서브넷별 Mermaid 다이어그램 export
  security_rules.go           Security List ingress/egress 규칙 테이블 뷰 (`v`) + export
  picker.go                   피커 오버레이 (region/action/bastion/resource 공용 — `f`/`:`가 resource 검색을 연다)
  compartment.go              breadcrumb, 루트 이름 조회, 리전 목록 조회
  bastion.go                  Bastion 세션 생성/폴링, SSH 명령 조립 (미검증 — 위 참고)
  paginate.go                 OpcNextPage 전체 순회 헬퍼
```

**사이드바(`sidebar.go`)는 존재하지 않는다** — 위 "사이드바 — 도입했다가 완전히 제거됨" 절 참고.

## 테스트 방법 (기록)

실제 `~/.oci/config`의 프로파일로 pty(`python3 pty.fork()`)를 통해 바이너리를 직접 구동하고 키 입력을 보내며 검증했다. `$JCID` 환경변수(개인 테스트 컴파트먼트 OCID)에 있는 `web11`/`web22` 인스턴스로 start/stop 왕복까지 실제로 실행하고 원상복구까지 확인함 — mock 없이 라이브 API로 검증하는 것이 이 프로젝트의 테스트 방식이다. 새 쓰기 액션을 추가할 때도 이 패턴을 따를 것(단, 실행 후 반드시 원상복구까지 확인).

**색상/레이아웃 버그는 `pyte`(파이썬 터미널 에뮬레이터)까지 동원해서 검증했다.** 단순히 렌더링된 문자열에 ANSI 코드가 "있냐 없냐"만 보면 안 되는 이유: bubbletea 실행 시 터미널이 커서 위치 질의(`\x1b[6n`)·배경색 질의(`\x1b]11;?`) 응답을 안 주면 첫 프레임을 못 그리고 멈춰있는다(pty 드라이버가 이 두 질의에 응답해줘야 함). 그리고 `lipgloss.NewStyle()`로 만든 패키지 레벨 스타일 변수(`selStyle` 등)는 **패키지 초기화 시점의 렌더러에 색이 바인딩**되므로, 테스트 안에서 나중에 `lipgloss.SetDefaultRenderer()`를 불러도 이미 만들어진 스타일엔 소급 적용되지 않는다 — 색 관련 로직은 `pyte.Screen`으로 실제 셀의 `fg`/`bg`를 찍어보는 것이 가장 확실하다 (STATE/NODE/EDITION 색, export 성공/실패 메시지 색 검증에 이 방법을 씀). 유닛 테스트에서 색을 검증할 땐 `lipgloss.SetColorProfile(termenv.TrueColor)`로 강제해야 한다 — `go test`엔 실제 TTY가 없어서 lipgloss가 기본적으로 색을 꺼버린다. 구조적 정확성(오프셋 계산 등)만 볼 때는 색 없이 `ansi.Strip()`으로 비교해도 충분.

**여러 실제 프로파일(`WYD`, `DEFAULT` 등)을 넘나들며 검증했다.** 한 프로파일(테넌시)에 원하는 실측 케이스가 없으면(예: RAC 2노드, Data Guard, 특정 EDITION) 다른 프로파일에서 찾는다 — `oci iam compartment list --compartment-id-in-subtree true`로 전체 컴파트먼트를 훑거나, 리전을 넘나드는 리소스(Exadata 등)를 찾을 땐 `oci search resource structured-search`(리전당 1콜로 테넌시 전체 컴파트먼트 검색, `--compartment-id-in-subtree`가 안 먹는 리소스 타입 list API보다 훨씬 빠름)로 여러 리전을 돌려본다.

**동시성 코드(goroutine fan-out)는 `go test -race`로 데이터 레이스를 확인한다.** 행별로 goroutine을 fan-out하는 패턴(DB System/Instance/Exadata 로딩 병렬화)을 추가한 뒤엔 반드시 `-race` 플래그로 한 번 더 돌려볼 것 — 컴파일은 되고 로직도 맞아 보여도 공유 상태에 락 없이 쓰면 조용히 데이터 레이스가 날 수 있다.
