# toci — Go 구현 계획

OCI용 터미널 UI. 공식 Go SDK를 백엔드로 삼아 인증·서명·페이지네이션·재시도를 전부 위임하고, 개발 노력을 UI와 워크플로에 집중한다.

---

## 1. 전제 조건 정리

### 이점

| 항목 | 상태 |
| --- | --- |
| 공식 SDK | **`github.com/oracle/oci-go-sdk/v65`** — Oracle 유지보수 |
| 서명 | SDK 내장. 직접 구현 불필요 |
| 페이지네이션 | `OpcNextPage` 응답 필드 → 다음 요청에 전달, 패턴 통일 |
| 재시도 | `common.DefaultRetryPolicy()` 제공 |
| 인증 | API 키 / 세션 토큰 / Instance Principal / Resource Principal 전부 지원 |

Rust 계획에서 1순위 위험으로 잡았던 서명 구현이 통째로 사라진다. M0가 필요 없다.

### 비용

- SDK 구조체가 리소스마다 제각각이다. 제네릭 테이블 매핑 레이어를 직접 설계해야 한다.
- 바이너리 크기가 Rust 대비 크다. oci-go-sdk 전체를 링크하면 수십 MB. → 사용 서비스 패키지만 import해 억제한다.
- SDK가 리소스마다 별도 클라이언트 타입을 요구한다 (`core.VirtualNetworkClient`, `core.ComputeClient`, `identity.IdentityClient`...). 클라이언트 팩토리가 필요하다.

---

## 2. 아키텍처

```
┌─────────────────────────────────────────┐
│  ui/          bubbletea 모델, 키맵       │
├─────────────────────────────────────────┤
│  app/         상태 머신, 뷰 스택         │
├─────────────────────────────────────────┤
│  registry/    Resource 인터페이스 구현   │  ← 리소스 추가 지점
├─────────────────────────────────────────┤
│  clients/     SDK 클라이언트 팩토리      │
├─────────────────────────────────────────┤
│  oci-go-sdk   (외부)                     │
└─────────────────────────────────────────┘
```

### 핵심 설계 결정: 인터페이스로 리소스를 추상화

Rust 계획은 `serde_json::Value` + JSON 포인터로 타입을 우회했다. Go에서는 SDK가 이미 타입을 주므로 그것을 살리되, 인터페이스로 균일하게 감싼다.

```go
type Resource interface {
    Key() string                      // ":vcn"
    Label() string                    // "VCNs"
    Columns() []Column
    List(ctx context.Context, s Scope) ([]Row, string, error)  // rows, nextPage, err
    Detail(ctx context.Context, id string) (any, error)
    Actions() []Action
}

type Column struct {
    Header string
    Width  int
    Get    func(Row) string          // 타입 안전한 접근자
}

type Row struct {
    ID   string
    Name string
    Raw  any                         // SDK 구조체 원본. 상세 뷰/YAML 출력용
}
```

`Get`이 클로저이므로 컴파일 타임에 필드명이 검증된다. Rust 계획의 JSON 포인터 오타 위험이 여기서는 없다.

리소스 하나 추가 비용:

```go
type VcnResource struct{ client core.VirtualNetworkClient }

func (r VcnResource) Columns() []Column {
    return []Column{
        {"NAME", 30, func(row Row) string {
            return deref(row.Raw.(core.Vcn).DisplayName)
        }},
        {"CIDR", 18, func(row Row) string {
            return deref(row.Raw.(core.Vcn).CidrBlock)
        }},
        {"STATE", 12, func(row Row) string {
            return string(row.Raw.(core.Vcn).LifecycleState)
        }},
    }
}
```

타입 단언이 반복되는 게 거슬리면 제네릭으로 정리할 수 있다:

```go
type TypedResource[T any] struct {
    columns []TypedColumn[T]
    lister  func(context.Context, Scope) ([]T, string, error)
}
```

Go 1.18+ 제네릭으로 단언을 걷어내되, 인터페이스 경계에서는 결국 `any`로 돌아온다. **초기에는 타입 단언 방식으로 가고, 리소스가 10종을 넘으면 그때 제네릭으로 리팩터링**한다. 미리 추상화하면 어떤 축이 실제로 반복되는지 모르는 상태에서 잘못된 추상화를 고정하게 된다.

---

## 3. SDK 사용 패턴

### 프로파일 로드

```go
provider := common.CustomProfileConfigProvider("", profileName)
// 또는 세션 토큰
provider, err := auth.NewSessionTokenProvider(configPath, profileName)
```

`--profile` 플래그와 `OCI_CLI_PROFILE` 환경변수 양쪽을 지원한다. 기존 `~/.oci/config`를 그대로 재사용하므로 별도 설정이 필요 없다.

### 클라이언트 팩토리

리전 전환 시 모든 클라이언트를 재생성해야 한다. 캐싱하되 리전 키로 관리한다.

```go
type Factory struct {
    provider common.ConfigurationProvider
    mu       sync.Mutex
    cache    map[string]any        // "region:type" → client
}
```

### 페이지네이션

```go
req := core.ListVcnsRequest{CompartmentId: &compID}
if page != "" { req.Page = &page }
resp, err := client.ListVcns(ctx, req)
next := ""
if resp.OpcNextPage != nil { next = *resp.OpcNextPage }
```

패턴이 모든 List 호출에 동일하다. 헬퍼로 한 번만 감싸면 된다.

---

## 4. 컴파트먼트 트리

AWS의 (리전 × 계정) 2축이 (리전 × 컴파트먼트 트리)가 된다. taws에 없는 UI 축.

```go
type CompartmentNode struct {
    ID       string
    Name     string
    Parent   *CompartmentNode      // Go에서는 부담 없음
    Children []*CompartmentNode
}
```

Rust 계획에서는 소유권 때문에 부모 포인터를 피하고 인덱스 스택으로 우회했지만, Go에서는 그냥 양방향 포인터를 둔다. **이것이 Go를 택하는 실질적 이유 중 하나다.**

`ListCompartments`에 `CompartmentIdInSubtree = true`를 주면 전체를 한 번에 받아 트리를 재구성할 수 있다. 단 테넌시 레벨 `inspect` 권한이 필요하다. 권한이 없을 때 403 대신 빈 결과가 오는 경우가 있으므로, 실패 시 "현재 컴파트먼트만" 모드로 폴백하고 상태바에 표시한다.

---

## 5. 스택

```
github.com/oracle/oci-go-sdk/v65      // 사용 서비스 패키지만 import
github.com/charmbracelet/bubbletea    // TUI 프레임워크
github.com/charmbracelet/bubbles      // table, textinput, viewport
github.com/charmbracelet/lipgloss     // 스타일링
github.com/sahilm/fuzzy               // 퍼지 매칭
github.com/spf13/cobra                // CLI 플래그
gopkg.in/yaml.v3                      // 상세 뷰 YAML
```

bubbletea는 Elm 아키텍처(Model-Update-View)라 ratatui의 즉시 모드와 사고방식이 다르다. 비동기 API 호출을 `tea.Cmd`로 감싸 메시지로 되돌리는 패턴에 익숙해지는 것이 초기 학습 포인트다.

```go
func fetchVcns(client core.VirtualNetworkClient, compID string) tea.Cmd {
    return func() tea.Msg {
        resp, err := client.ListVcns(...)
        if err != nil { return errMsg{err} }
        return vcnsLoadedMsg{resp.Items}
    }
}
```

### 바이너리 크기 억제

```bash
go build -ldflags="-s -w" -trimpath
```

import를 사용 패키지로 제한하는 것이 가장 효과가 크다. `oci-go-sdk` 전체를 blank import하면 안 된다.

---

## 6. 마일스톤

### M1 — 읽기 전용 골격 (1~2일)

- `common.CustomProfileConfigProvider`로 프로파일 로드
- 클라이언트 팩토리
- `Resource` 인터페이스 + 3종: 컴파트먼트, VCN, 서브넷
- bubbles/table 뷰, `j/k`, `Enter` 상세, `Esc`, `Ctrl-c`
- 페이지네이션 헬퍼

Rust 계획의 M0(서명 검증)가 없고 M1도 짧다. SDK가 절반을 해준다.

### M2 — 실사용 최소치 (2~3일)

- 컴파트먼트 트리 네비게이션
- `:` 리소스 피커 + 퍼지 자동완성
- `/` 로컬 퍼지 필터
- 리소스 추가: Compute 인스턴스, 라우트 테이블, 시큐리티 리스트/NSG, LB, DRG
- `R` 새로고침, 리전 전환

### M3 — 액션 (2~4일)

- **`--readonly` 기본값 ON.** 쓰기는 명시적 `--write` 필요
- 인스턴스 start/stop (리소스명 타이핑 확인)
- Bastion 세션 생성 → SSH 터널
- 터미널 제어권 양보 후 복구 (`stty sane` + stdin flush)

### M4 — 배포

- GoReleaser로 크로스 컴파일 + Homebrew tap 자동 생성
- Docker 이미지 (선택)

---

## 7. 위험 요소

| 위험 | 대응 |
| --- | --- |
| SDK 구조체 불균일로 매핑 레이어가 비대해짐 | 리소스 10종까지는 단언 방식 유지, 이후 제네릭 리팩터링 |
| 바이너리 크기 | import 범위 제한 + `-ldflags="-s -w"` |
| bubbletea 비동기 패턴 학습 | M1에서 VCN 조회 하나로 패턴 확립 후 복제 |
| 프로덕션 오조작 | readonly 기본값. 쓰기 액션은 확인 프롬프트 |

`--readonly`는 나중에 붙이는 기능이 아니라 **처음부터 기본값**이다. 클라이언트 테넌시를 다루는 환경에서는 특히 그렇다.

---

## 8. 평가

**적합한 이유:** 공식 SDK가 서명·재시도·페이지네이션·인증 4가지를 해결한다. 컴파트먼트 트리 같은 그래프성 자료구조에서 소유권과 씨름할 일이 없다. GoReleaser 배포 파이프라인이 성숙하다. 무엇보다 **프로토타입까지 도달하는 시간이 짧다** — 주말 두어 번이면 실사용 가능한 물건이 나온다.

**부적합한 이유:** 바이너리가 크다. 타입 매핑 레이어를 직접 설계해야 한다. 생태계 기여 관점에서 새로 만드는 것이 없다 — SDK가 이미 있으므로 `oci-signer` 같은 빈 자리를 메우는 성취가 없다.

**권장:** **이 프로젝트의 1순위 선택.** 먼저 Go로 만들어 어떤 리소스와 워크플로가 실제로 필요한지 확정한다. 설계가 굳은 뒤 Rust 재구현을 고려하면, 두 번째 구현에서는 Rust 학습 자체에 집중할 수 있다.

다만 **`oci-signer` 크레이트만은 Go 계획과 병행해 Rust로 만들 가치가 있다.** 독립적이고, 작고, 생태계에 없다. TUI 본체와 분리된 작업이므로 둘 중 하나를 포기할 필요가 없다.
