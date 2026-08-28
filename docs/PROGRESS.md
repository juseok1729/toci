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
- **`:` 리소스 피커** — 최초엔 `sahilm/fuzzy`로 실시간 필터링되는 오버레이였으나, 이후 세션에서 **왼쪽 사이드바 리소스 트리**로 교체됨 (아래 "계획에 없던, 추가로 구현한 기능" 참고). `internal/app/picker.go`의 `picker` 타입은 이제 리전 피커/액션 피커/Bastion 피커 세 가지 용도로만 쓰인다 (`pickerResource`는 제거됨).
- **`/` 로컬 퍼지 필터** — 이미 불러온 행(`m.rows`)에 대해 이름 기준 클라이언트 사이드 필터링. `Esc`로 되돌리기(편집 시작 시점 값 백업), `Enter`로 확정.
- **리소스 9종**: Compartment, Instance, VCN, Subnet, Route Table, Security List, NSG, DRG, Load Balancer.
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

### 왼쪽 사이드바 리소스 트리 (`internal/app/sidebar.go`)
- `:` 를 눌렀을 때 뜨던 오버레이 피커(M2에서 만든 `pickerResource`)를 없애고, 항상 화면에 보이는 왼쪽 사이드바 트리로 교체했다.
- `:` 는 이제 팝업을 여는 대신 사이드바로 포커스를 옮긴다(`modeSidebar`) — `j`/`k`·↑↓로 트리 이동, `Enter` 선택, `Esc` 취소. 상시 표시되는 패널이라 현재 리소스가 항상 하이라이트된다.
- 리소스 종류에 category 필드가 없어서, 사이드바 전용으로 4개 카테고리(Compartments/Compute/Network/Load Balancing)를 `resourceCategories`에 하드코딩 — 리소스가 늘어나 분류가 안 맞으면 이 슬라이스만 고치면 된다.
- region/action/bastion 피커는 여전히 팝업(`modePicker`) 방식 그대로 둠 — 자주 안 쓰는 짧은 흐름이라 트리로 바꿀 이유가 없었음.
- `WindowSizeMsg` 핸들러에서 테이블/디테일 너비를 `sidebarWidth`(24)만큼 줄여 사이드바와 겹치지 않게 함.
- **컴파트먼트 경로 표시**: 사이드바의 "Compartments" 리프 밑에 현재 위치한 컴파트먼트 경로(root → ... → 현재)를 항상 트리로 함께 그린다. 새 상태 없이 기존 `m.compPath`(breadcrumb에도 쓰던 값)를 재사용했고, 마지막 항목(현재 컴파트먼트)만 강조 스타일로 구분한다.

### Instance 목록 스펙/메트릭 컬럼 (`internal/registry/instance.go`, `instance_metrics.go`)
- Instance 테이블에 `OCPU`, `MEM(GB)`(스펙), `CPU%`, `MEM%`(메트릭) 컬럼을 추가했다.
- **스펙**: 별도 API 호출 없이 `ListInstances` 응답의 `Instance.ShapeConfig`(`Ocpus`, `MemoryInGBs`)를 그대로 사용.
- **메트릭**: OCI Monitoring(`oci_computeagent` 네임스페이스)에서 최근 10분 평균값을 조회. 인스턴스별이 아니라 **컴파트먼트당 2번**의 `SummarizeMetricsData` 호출(CPU 1번, Mem 1번)로 한번에 조회 후 응답의 `resourceId` 디멘션으로 매칭한다 — 인스턴스 N개당 N번 호출하는 방식은 피함.
  - 조회 실패(모니터링 권한 없음, 컴퓨트 에이전트 미설치 등)는 조용히 `-`로 표시되고 인스턴스 목록 자체는 정상 표시된다 — 별도 에러 처리 없음.
  - 테넌시/컴파트먼트에 `read metrics` IAM 권한이 없으면 CPU%/MEM%는 계속 `-`로만 보인다.
- **Storage(부트볼륨 크기)는 스킵** — `Instance` 응답에 없고, 인스턴스마다 별도 API 2콜(boot volume attachment 조회 + boot volume 상세 조회)이 필요해 N+1 비용이 크다. 필요해지면 추가 가능.
- `Row.Raw`는 기존 `core.Instance` 대신 `instanceRow{core.Instance, Metrics instanceMetrics}`로 감쌌다 — `Columns()`의 타입 단언만 이 파일 안에서 바뀌었고, `Row.Raw`를 참조하는 다른 코드(`bastion.go` 등)는 없어서 영향 없음.
- `internal/clients/factory.go`에 `Monitoring(region)` 클라이언트 캐시를 추가 (기존 identity/vcn/compute/bastion/lb와 동일한 캐싱 패턴).

### 사이드바 재구성 + 버그 3종

- **카테고리 재편**: `Compute`/`Network`/`Load Balancing`을 하나로 합쳐 **`VCN-scoped`**로, `DRG`는 **`Global-scoped`**로 분리. 어떤 리소스가 VCN에 종속되는지(서브넷 유무)를 기준으로 나눴다. `resourceCategories`의 `vcnScoped` 플래그가 유일한 근거지 — `isVcnDependent()`가 여기서 파생된다.
- **길이 자동 조절 + 최소폭 버그**: 긴 컴파트먼트 이름이 줄바꿈되던 문제 — 원인은 `lipgloss.Style.Width()`가 이미 패딩 포함 폭이라 텍스트를 `width-padding`에서 자르는데, 순수 텍스트 폭만 넘겨서 항상 패딩만큼(2칸) 모자라게 잘렸던 것. `.Width(sidebarContentWidth(m) + 2)`로 고침.
  - 추가로, 좁은 터미널에서 사이드바 최소폭(20)과 메인 영역 최소폭(30)이 서로 독립적으로 고집하다 합이 터미널 폭을 넘는 경우 발견 → `sidebarAbsFloor`(8)/`mainAbsFloor`(10) 같은 "진짜 최소치"를 따로 둬서 좁을 땐 "선호값"(20/30)을 포기하도록 함.
- **Compartments 리셋**: 사이드바에서 Compartments를 다시 선택하면 항상 테넌시 루트로 돌아가 최상위부터 다시 드릴다운하게 함 (`switchToRootCompartments`) — 안 그러면 리프 컴파트먼트에 있을 때 재선택해도 빈 화면만 반복됐음.
- **컴파트먼트 진입 시 자동 리다이렉트**: 하위 컴파트먼트가 없는 리프에 진입하면 자동으로 **VCN** 목록으로 전환 (`m.autoRedirect`) — 빈 Compartments 화면 대신 뭔가 보여주기 위함. 사이드바에서 수동으로 Compartments를 다시 고르는 경우엔 이 자동전환이 발동하지 않도록 별도 플래그로 구분.

### VCN 스코프 필터링 일반화 + DB 리소스 3종 추가

- VCN 목록에서 **`i`** → 그 VCN을 필터로 지정(`selectVcnFilter`)하고 사이드바가 열림. 이후 VCN-scoped 카테고리 안에서 리소스를 옮겨다녀도(Subnet↔Instance 등) 필터가 유지되고, Compartments/DRG처럼 스코프 밖으로 나가면 자동 해제.
- **DB Systems**(`db-system`), **Autonomous DBs**(`adb`), **Exadata VM Clusters**(`exadata`, 실제로는 `CloudVmCluster` — 물리 인프라 계층인 `CloudExadataInfrastructure`는 SubnetId가 없어 VCN 종속이 아니라서 제외) 3종 추가. 전부 자체 `SubnetId` 필드가 있어서 Instance처럼 VNIC 조인이 필요 없음.
- Instance의 VCN 필터링에 쓰던 `instanceIDsInVcn`을 리팩터링해서 `registry.InstanceSubnetIDs`(인스턴스ID→서브넷ID 전체 맵)를 export — 나중에 Mermaid 다이어그램에서 재사용.

### Instance 컬럼 재정렬 + STATE 배경색 (제일 오래 걸린 버그 사냥)

컬럼 순서를 `NAME, IP(PUB/PRI), SHAPE, OCPU, MEM(GB), USAGE(CPU/MEM %), DOMAIN(AD/FD), STATE`로 정리하고, STATE의 RUNNING/STOPPED에 배경색(초록/빨강, 커서 있는 행은 더 어둡게)을 입히는 과정에서 구조적 버그를 세 번 연달아 만났다:

1. **셀 값에 ANSI를 직접 심는 방식은 안 된다** — bubbles 테이블이 `go-runewidth`로 셀을 자르는데, 이 라이브러리가 ANSI 이스케이프 바이트를 전부 "보이는 글자"로 세서 색칠된 값이 중간에 잘리고 리셋 코드까지 날아가 색이 뒤 칸으로 번짐.
2. **컬럼 폭을 넉넉히 줘서 안 잘리게 해도, 리셋을 `\x1b[0m`(전체 리셋)으로 하면 선택된(커서) 행의 하이라이트가 STATE 이후 칸부터 끊김** — 리셋이 "내가 선택 행 스타일 안에 있다"는 걸 모르기 때문.
3. **최종 해법**: 색칠을 셀 값이 아니라 **테이블이 다 그려진 뒤 최종 문자열에 후처리**로 입힘 (`internal/app/state_color.go`). `charmbracelet/x/ansi`(lipgloss가 이미 의존 중이던 걸 직접 의존성으로 승격)의 `ansi.Cut`으로 STATE 컬럼 구간만 잘라 배지를 끼워넣는데, 이 함수는 자르는 지점 이전에 열려있던 스타일(선택 행 하이라이트)을 뒷부분에 그대로 이어붙여줘서 문제가 해결됨.
   - **컬럼 폭 자동 축소와의 상호작용 버그**: 이후 모든 컬럼에 fit-to-content(`fitColumns`, 창 크기 비례 축소 포함)를 적용하면서, `colorizeInstanceState`가 STATE 위치를 계산할 때 여전히 `registry.Column`의 **선언된(고정) 폭**을 쓰고 있어서 실제 렌더 폭과 어긋나 색이 안 나오는 회귀가 생겼다 — `m.current().Columns()` 대신 `m.table.Columns()`(bubbles가 실제로 렌더링에 쓴 폭)를 쓰도록 고쳐서 해결.
   - 검은 글씨(`Color("0")`)에 `Bold(true)`를 같이 쓰면 터미널이 "bold=밝게"로 해석해서 회색으로 보이는 것도 발견 — 256색 팔레트의 순수 검정(`Color("16")`)으로 바꿔서 해결.
- Bold=true, fg=검정(16), bg=초록(2)/빨강(1), 커서 행은 더 어두운 초록(28)/빨강(88).

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
- **`m`**(VCN 필터가 걸려있을 때만): 그 VCN의 서브넷별로 Instance/DB System/ADB/Exadata를 그룹핑하고, 그 VCN에 붙어있는 DRG까지 포함해서 Mermaid **`architecture-beta`**(mermaid.js의 클라우드 아키텍처 전용 문법) 텍스트를 `.mmd` 파일로 저장 (`internal/app/diagram.go`). 여러 리소스를 새로 fetch해야 해서 비동기(`tea.Cmd`)로 처리.
  - DRG는 VCN 자체에 붙는 거라(서브넷 소속이 아님) VCN 그룹 밖에 별도 `service`로 두고, VCN 그룹 안의 `junction`을 앵커 삼아 연결. `ListDrgAttachments(vcnId=...)`로 이 VCN에 붙은 DRG만 걸러냄 (컴파트먼트의 DRG 전부를 무조건 넣지 않음).
  - 아이콘은 `cloud`/`database`/`disk`/`internet`/`server` 5종 내장분만 외부 아이콘팩 등록 없이 쓸 수 있어서, Instance→`server`, DB 3종→`database`, DRG→`internet`로 매핑.
  - **한 번 `graph TD`+`subgraph`로 되돌렸다가 다시 `architecture-beta`로 복귀했다.** 처음엔 실제 mermaid.js 파서로 문법 검증까지 통과했는데 **Notion에 붙여넣으니 렌더링 에러**가 났다. 원인은 diagram 타입 자체를 Notion이 못 알아본 게 아니라 — `architecture-beta`의 `[label]`은 따옴표 없는 텍스트라서, OCI 리소스 이름처럼 하이픈이 들어간 라벨(`wyd-logistics-drg` 등)을 Notion이 쓰는 mermaid 버전 문법이 못 받아들였던 것(mermaid 공식 예제가 전부 `API`/`Database`처럼 하이픈 없는 단어인 이유가 이거였다). 그래서 일단 라벨을 항상 따옴표로 감싸는 `graph TD`+`subgraph`로 바꿨는데, 사용자가 Notion에서 architecture-beta 자체는 지원된다는 걸 직접 확인해줘서 — **라벨만 따옴표로 감싸는 걸로(`["wyd-logistics-drg"]`) 고쳐서 `architecture-beta`로 다시 복귀**했다.
  - 문법 검증은 매 단계 실제 mermaid.js 파서로 했다(Node+JSDOM `mermaid.parse()`) — 라이브 데이터(DRG 있음/없음)로 만든 `.mmd`가 전부 `diagramType: "architecture"`로 정상 파싱되는 것 확인. 완전한 SVG 렌더(`mmdc`)까지는 샌드박스에 Chromium 의존 시스템 라이브러리(`libasound.so.2`)가 없어서(sudo 필요) 못 봤지만, 최종 버전은 아직 Notion에서 실사용 확인 전 — 다음 세션에서 문제 생기면 여기부터 볼 것.
- xlsx는 stdlib에 없어서 새 의존성이 필요해 스킵 — BOM CSV로 엑셀 호환 문제는 이미 해결되니 필요해지면 그때 추가.

### Security List 규칙 테이블 뷰 (`v`)

Security List에서 `v`를 누르면 ingress/egress 규칙을 YAML 대신 `lipgloss/table`(lipgloss에 이미 포함된 서브패키지, 새 의존성 아님)로 만든 표로 보여줌. Protocol 번호(`6`→TCP 등)를 이름으로, 포트도 읽기 쉽게 변환. 기존 Enter(YAML 상세)는 그대로 두고 별도 키로 추가.

### 상태 메시지 색상

`m.statusMsg`가 "exported"/"diagram written"으로 시작하면 초록, "failed"/"error"를 포함하면 빨강, 나머지는 기존 회색 — `renderStatusMsg()` 하나로 상태줄과 모드-디테일 화면(Security List 규칙 뷰) 양쪽에서 재사용.

## 계획에 없던, 구현하며 발견한 이슈

**bubbles table/viewport의 내부 상태 버그**: 기존 `table.Model`에 `SetRows()`로 더 적은/다른 행을 밀어넣으면, 이전 커서·스크롤 오프셋(YOffset)이 새 행 수와 안 맞아 `viewport.visibleLines()`에서 `slice bounds out of range` 패닉이 난다 (bubbles v1.0.0 기준, `clamp()`가 `low > high`일 때 값을 스왑하는 구현 때문에 top > bottom인 슬라이스가 만들어짐).

**대응**: 리소스 전환·필터 적용·새로고침 등 행 집합이 바뀌는 모든 지점에서 기존 테이블을 **mutate하지 않고 `newTable()`로 새로 생성**한다 (`internal/app/model.go`의 `refreshTable()`). 이 패턴을 벗어나면(즉 기존 `table.Model`에 직접 `SetRows`/`SetCursor`를 호출하면) 같은 클래스의 패닉이 재발할 수 있으니, 새 코드에서도 이 규칙을 지킬 것.

## 아직 안 한 것

### M3 나머지
- Bastion 세션/SSH 코드는 구현됐지만 **실제 세션으로 검증되지 않았다** — 자세한 내용과 재개 방법은 위 "M3 (일부) — Bastion 세션 → SSH" 절 참고. 가장 먼저 할 일은 Bastion 하나를 확보해서 `SshMetadata`의 실제 키/값 포맷부터 확인하는 것.
- 세션 정리(session delete)는 구현 안 함 — 세션은 TTL(현재 1800초 고정)로 자동 만료되므로 없어도 동작엔 지장 없지만, 명시적으로 끝내고 싶을 때를 위해 `d`(delete session) 같은 키를 나중에 추가할 수 있음.

### M4 — 배포
- GoReleaser 크로스 컴파일 + Homebrew tap
- Docker 이미지 (선택)

## 파일 지도

```
cmd/toci/main.go              cobra 진입점, --profile/--region/--write
internal/clients/factory.go   리전별 SDK 클라이언트 캐시 (identity/vcn/compute/bastion/lb/monitoring/database)
internal/registry/
  resource.go                 Resource, Actionable, Column, Row, Scope 인터페이스/타입 (Scope.VcnID 포함)
  compartment.go ~ lb.go      리소스 9종 구현체
  db_system.go / adb.go / exadata.go   DB Systems / Autonomous DBs / Exadata VM Clusters (전부 VCN-scoped)
  instance_metrics.go         Instance CPU%/MEM% 조회 (Monitoring, 컴파트먼트당 2콜)
  instance_ip.go              Instance Public/Private IP 조회 (VNIC 조인)
  instance_vcn_filter.go      VCN 필터용 서브넷/인스턴스 조인 헬퍼 + InstanceSubnetIDs(export, 다이어그램용)
  registry.go                 All() — UI에 노출되는 리소스 순서
internal/app/
  model.go                    bubbletea Model — 상태머신 본체 (Update/View)
  sidebar.go                  왼쪽 사이드바 리소스 트리(Compartments/VCN-scoped/Global-scoped) + 컴파트먼트 경로 표시
  state_color.go              Instance STATE 배경색 후처리 (ansi.Cut 기반, 렌더 후 색칠)
  help.go                     스페이스바 which-key 팝업 (우측하단 오버레이)
  export.go                   CSV export 공통 로직 (UTF-8 BOM)
  diagram.go                  VCN 서브넷별 Mermaid 다이어그램 export
  security_rules.go           Security List ingress/egress 규칙 테이블 뷰 (`v`) + export
  picker.go                   피커 오버레이 (region/action/bastion 공용, resource 피커는 제거됨)
  compartment.go              breadcrumb, 루트 이름 조회, 리전 목록 조회
  bastion.go                  Bastion 세션 생성/폴링, SSH 명령 조립 (미검증 — 위 참고)
  paginate.go                 OpcNextPage 전체 순회 헬퍼
```

## 테스트 방법 (기록)

실제 `~/.oci/config`의 프로파일로 pty(`python3 pty.fork()`)를 통해 바이너리를 직접 구동하고 키 입력을 보내며 검증했다. `$JCID` 환경변수(개인 테스트 컴파트먼트 OCID)에 있는 `web11`/`web22` 인스턴스로 start/stop 왕복까지 실제로 실행하고 원상복구까지 확인함 — mock 없이 라이브 API로 검증하는 것이 이 프로젝트의 테스트 방식이다. 새 쓰기 액션을 추가할 때도 이 패턴을 따를 것(단, 실행 후 반드시 원상복구까지 확인).

**색상/레이아웃 버그는 `pyte`(파이썬 터미널 에뮬레이터)까지 동원해서 검증했다.** 단순히 렌더링된 문자열에 ANSI 코드가 "있냐 없냐"만 보면 안 되는 이유: bubbletea 실행 시 터미널이 커서 위치 질의(`\x1b[6n`)·배경색 질의(`\x1b]11;?`) 응답을 안 주면 첫 프레임을 못 그리고 멈춰있는다(pty 드라이버가 이 두 질의에 응답해줘야 함). 그리고 `lipgloss.NewStyle()`로 만든 패키지 레벨 스타일 변수(`selStyle` 등)는 **패키지 초기화 시점의 렌더러에 색이 바인딩**되므로, 테스트 안에서 나중에 `lipgloss.SetDefaultRenderer()`를 불러도 이미 만들어진 스타일엔 소급 적용되지 않는다 — 색 관련 로직은 `pyte.Screen`으로 실제 셀의 `fg`/`bg`를 찍어보는 것이 가장 확실하다 (STATE 배경색, export 성공/실패 메시지 색 검증에 이 방법을 씀). 구조적 정확성(오프셋 계산 등)만 볼 때는 색 없이 `ansi.Strip()`으로 비교해도 충분.
