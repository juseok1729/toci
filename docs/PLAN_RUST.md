# toci — Rust 구현 계획

OCI용 터미널 UI. taws(Rust/Ratatui)를 참조 구현으로 삼되, 공식 SDK 부재라는 근본적 제약을 전제로 설계한다.

---

## 1. 전제 조건 정리

### 제약

| 항목 | 상태 |
| --- | --- |
| 공식 Rust SDK | **없음** (Java/Python/Go/TS/.NET/Ruby/PHP만 제공) |
| `oci-sdk` (digital-divas) | 0.3.0, 2022년 이후 정체. Identity 일부만 |
| `oci-api` (GoCoder7) | 비교적 최근이나 Email Delivery / Object Storage / KMS 한정 |
| 결론 | **HTTP 서명 계층부터 직접 구현** |

taws는 `aws-sigv4`(공식 aws-sdk-rust 구성요소)에 서명을 위임한다. 이 프로젝트에는 대응물이 없으므로, taws가 공짜로 얻은 것을 직접 만들어야 한다. 반대로 이것이 이 프로젝트의 차별점이기도 하다 — 서명 크레이트 자체가 생태계의 빈 자리다.

### 이점

- 의존성이 taws보다 가볍다. aws-sdk-rust는 서비스별 크레이트가 붙어 빌드가 무거워지지만, 여기서는 `reqwest` 하나로 끝난다.
- OCI 인증은 API 키 / 세션 토큰 2종뿐. taws의 7단계 credential chain이 2단계로 축소된다.
- `Option<T>` 강제가 optional 필드가 난무하는 OCI 응답에서 실제로 버그를 줄인다.
- musl 정적 링크 단일 바이너리. Go보다 산출물이 작다.

---

## 2. 아키텍처

```
┌─────────────────────────────────────────┐
│  ui/          Ratatui 렌더링, 키맵       │
├─────────────────────────────────────────┤
│  app/         상태 머신, 뷰 스택         │
├─────────────────────────────────────────┤
│  registry/    ResourceDef 테이블         │  ← 리소스 추가 지점
├─────────────────────────────────────────┤
│  client/      페이지네이션, 재시도       │
├─────────────────────────────────────────┤
│  signer/      draft-cavage 서명  ★분리★  │  ← 별도 크레이트화
├─────────────────────────────────────────┤
│  config/      ~/.oci/config INI 파싱     │
└─────────────────────────────────────────┘
```

`signer/`는 처음부터 워크스페이스 내 독립 크레이트로 둔다. 나중에 `oci-signer`로 crates.io에 publish.

### 핵심 설계 결정: 타입을 정의하지 않는다

리소스마다 `#[derive(Deserialize)]` 구조체를 만들면 20종 = 20번의 스키마 필사 작업이다. 대신 `serde_json::Value`로 받고 JSON 포인터로 컬럼을 뽑는다.

```rust
pub struct ResourceDef {
    pub key: &'static str,           // ":vcn"
    pub label: &'static str,         // "VCNs"
    pub service: Service,            // Service::Iaas
    pub path: &'static str,          // "/20160918/vcns"
    pub scope: Scope,                // Scope::Compartment
    pub columns: &'static [Column],
    pub actions: &'static [Action],
}

pub struct Column {
    pub header: &'static str,        // "CIDR"
    pub pointer: &'static str,       // "/cidrBlock"
    pub width: Constraint,
}
```

타입 안전성을 포기하는 대신 리소스 추가가 테이블 한 줄이 된다. TUI는 어차피 문자열로 렌더링하고, 상세 뷰는 원본 `Value`를 그대로 JSON/YAML로 출력하면 된다. taws가 51개 리소스 타입을 감당하는 것도 유사한 구조일 가능성이 높다.

트레이드오프: 오타난 JSON 포인터가 컴파일 타임에 안 잡힌다. → 각 `ResourceDef`에 대해 픽스처 응답으로 컬럼 추출을 검증하는 테스트를 둔다.

---

## 3. 서명 계층 (signer/)

OCI는 draft-cavage HTTP Signatures를 사용한다. SigV4보다 단순하다.

### signing string 구성

```
(request-target): get /20160918/vcns?compartmentId=ocid1...
date: Tue, 26 Aug 2026 04:35:33 GMT
host: iaas.ap-seoul-1.oraclecloud.com
```

POST/PUT/PATCH는 여기에 추가:

```
content-length: 137
content-type: application/json
x-content-sha256: <body의 SHA256을 base64>
```

### Authorization 헤더

```
Signature version="1",
  keyId="{tenancy}/{user}/{fingerprint}",
  algorithm="rsa-sha256",
  headers="(request-target) date host",
  signature="{RSA-SHA256 서명을 base64}"
```

`keyId`는 config의 세 값을 슬래시로 이은 형태. 파싱이 필요 없다.

### 구현 체크리스트

- [ ] PEM 개인키 로드 (`rsa` 크레이트, PKCS#1/PKCS#8 양쪽)
- [ ] passphrase 걸린 키 처리 (`pass_phrase` 필드)
- [ ] `Date` 헤더 RFC 2822 포맷 (OCI는 5분 skew 허용)
- [ ] GET/HEAD/DELETE와 body 있는 메서드 분기
- [ ] 세션 토큰 인증 (`security_token_file`) — `keyId="ST${token}"`
- [ ] 클럭 스큐 오류를 사용자에게 명확히 (`401` + 로컬 시각 표시)

**개발 순서상 여기가 1순위.** 서명이 안 되면 나머지가 전부 무의미하다. `oci iam region list`와 동일한 요청을 보내 200이 오는 것을 확인한 뒤 TUI에 착수한다.

---

## 4. 컴파트먼트 트리

AWS의 (리전 × 계정) 2축이 OCI에서는 (리전 × 컴파트먼트 트리)가 된다. taws에 없는 UI 축이 하나 늘어난다.

```rust
struct CompartmentNode {
    id: String,
    name: String,
    children: Vec<CompartmentNode>,   // 소유권 트리, 부모 참조 없음
}
```

부모 포인터를 넣고 싶어지지만 `Rc<RefCell<>>`로 가는 순간 코드가 지저분해진다. **경로는 `Vec<usize>` 인덱스 스택으로 별도 관리**한다. 이게 Rust에서 트리 UI를 다루는 가장 덜 아픈 방법이다.

`--compartment-id-in-subtree true`는 테넌시 레벨 `inspect` 권한을 요구한다. 권한이 없으면 403이 아니라 빈 결과가 오는 경우가 있으므로, 트리 조회 실패 시 "현재 컴파트먼트만" 모드로 자동 폴백하고 상태바에 표시한다.

---

## 5. 스택

```toml
[dependencies]
ratatui        = "0.29"
crossterm      = "0.28"
tokio          = { version = "1", features = ["rt-multi-thread", "macros"] }
reqwest        = { version = "0.12", features = ["json", "rustls-tls"], default-features = false }
serde          = { version = "1", features = ["derive"] }
serde_json     = "1"
serde_yaml     = "0.9"
rsa            = "0.9"
sha2           = "0.10"
base64         = "0.22"
rust-ini       = "0.21"
nucleo-matcher = "0.3"
clap           = { version = "4", features = ["derive"] }
anyhow         = "1"
tracing        = "0.1"
```

`rustls-tls`로 OpenSSL 의존을 끊어 musl 정적 링크를 쉽게 만든다. (단, 서명용 `rsa` 크레이트는 순수 Rust 구현이므로 문제없음)

---

## 6. 마일스톤

### M0 — 서명 검증 (0.5일)

TUI 없이 CLI 바이너리로 `GET /20160918/vcns` 성공시키기. 여기서 막히면 계획 재검토.

### M1 — 읽기 전용 골격 (2~3일)

- `~/.oci/config` 파싱, `--profile` 플래그
- `ResourceDef` 레지스트리 + 3종: 컴파트먼트, VCN, 서브넷
- 테이블 뷰, `j/k` 이동, `Enter` 상세(JSON), `Esc` 복귀, `Ctrl-c` 종료
- 페이지네이션 (`opc-next-page` 헤더 → `page` 파라미터)

### M2 — 실사용 최소치 (3~4일)

- 컴파트먼트 트리 네비게이션
- `:` 리소스 피커 + 퍼지 자동완성
- `/` 로컬 퍼지 필터
- 리소스 추가: Compute 인스턴스, 라우트 테이블, 시큐리티 리스트/NSG, LB
- `R` 새로고침, 리전 전환

### M3 — 액션 (3~5일)

- **`--readonly` 기본값 ON.** 쓰기 활성화는 명시적 `--write` 필요
- 인스턴스 start/stop (확인 프롬프트 필수)
- Bastion 세션 생성 → SSH 터널. 터미널 제어권 양보 구간
- `stty sane` + stdin flush로 복귀 시 터미널 복구

### M4 — 배포

- GitHub Actions 크로스 컴파일 (darwin arm64/x86_64, linux musl arm64/x86_64)
- `cargo publish`로 `oci-signer` 분리 배포
- Homebrew tap

---

## 7. 위험 요소

| 위험 | 대응 |
| --- | --- |
| 서명 구현이 예상보다 오래 걸림 | M0에서 조기 판단. 실패 시 Go 계획으로 전환 |
| 리소스별 엔드포인트 호스트가 제각각 | `Service` enum으로 호스트 템플릿 관리 (`iaas`, `identity`, `objectstorage`...) |
| 프로덕션 오조작 | readonly 기본값. 쓰기 액션은 리소스명 타이핑 확인 |
| 유지보수 부담 | 리소스 추가를 테이블 한 줄로 만들어 기여 장벽을 낮춤 |

`--readonly`는 나중에 붙이는 기능이 아니라 **처음부터 기본값**으로 둔다. 클라이언트 테넌시를 다루는 환경에서는 특히 그렇다. taws 커뮤니티에서도 "AWS 위에 미들웨어를 얹는 것 자체"에 대한 우려 — 잘못 해석된 명령이나 버그가 stateful 워크로드에 닿으면 복구가 어렵다는 지적이 제기됐다.

---

## 8. 평가

**적합한 이유:** 단일 바이너리 배포, `Option<T>`의 null 안전성, 가벼운 의존성 트리, 그리고 `oci-signer` 크레이트라는 명확한 생태계 기여 지점.

**부적합한 이유:** 공식 SDK 부재로 초기 비용이 Go 대비 크다. 컴파트먼트 트리 같은 그래프성 자료구조에서 소유권과 씨름하게 된다. 학습 곡선이 프로젝트 진행 속도를 지배할 수 있다.

**권장:** Go로 먼저 만들어 설계를 확정한 뒤, 두 번째 구현으로 Rust를 택하는 순서. 설계가 머릿속에 있는 상태에서 재구현하면 Rust 학습 자체에 집중할 수 있다. 단, `oci-signer`만은 지금 당장 Rust로 만들어 publish할 가치가 있다 — 독립적이고, 작고, 생태계에 없다.
