# toci — 실행 가이드

현재까지(M1~M2, M3 일부) 구현된 기능 기준.

## 빌드

```bash
go build -o toci ./cmd/toci
```

배포용 바이너리(계획서의 크기 억제 옵션):

```bash
go build -ldflags="-s -w" -trimpath -o toci ./cmd/toci
```

## 실행

`~/.oci/config`에 프로파일이 이미 있어야 한다 (OCI CLI와 동일한 설정 파일을 그대로 사용).

```bash
./toci                              # 프로파일: $OCI_CLI_PROFILE 또는 DEFAULT
./toci --profile ETEVERS          # 특정 프로파일
./toci --profile ETEVERS --region us-ashburn-1   # 리전 강제 지정 (기본: 프로파일의 region)
./toci --profile ETEVERS --write  # 쓰기 액션(인스턴스 start/stop) 활성화
```

기본은 **readonly** — `--write` 없이는 `a` 키를 눌러도 아무 것도 실행되지 않는다.

시작 시 테넌시 루트 컴파트먼트를 기준으로 Compartments 목록이 뜬다.

## 키 바인딩

| 키 | 동작 |
| --- | --- |
| `j` / `k` (또는 방향키) | 목록 위아래 이동 |
| `Enter` | Compartment 리소스: 하위 컴파트먼트로 진입 · 그 외 리소스: 상세(YAML) 보기 |
| `Esc` | 상세 보기 닫기 → (필터 중이면) 필터 해제 → (하위 컴파트먼트면) 상위로 복귀, 순서로 하나씩 처리 |
| `Tab` | 다음 리소스 종류로 순환 전환 |
| `:` | 리소스 피커 열기 — 타이핑하면 퍼지 필터링, `Enter`로 선택 |
| `/` | 현재 목록을 이름으로 로컬 필터링 — `Enter` 확정, `Esc` 취소(직전 값으로 복원) |
| `r` | 리전 피커 열기 (테넌시가 구독한 리전만 표시) |
| `R` | 현재 화면 새로고침 |
| `a` | (Instance 리소스 + `--write`일 때만) 선택한 행에 대한 액션 메뉴 — 액션 선택 후 **인스턴스 이름을 정확히 입력해야** 실행됨 |
| `s` | (Instance 리소스 + `--write`일 때만) 선택한 인스턴스에 Bastion으로 SSH 접속 — ⚠️ 아래 "Bastion SSH" 참고, 실사용 미검증 |
| `q` / `Ctrl-C` | 종료 |

## 지원 리소스

Compartments, Instances, VCNs, Subnets, Route Tables, Security Lists, Network Security Groups, DRGs, Load Balancers.

## 지원 액션 (Instance, `--write` 필요)

- **Start** — `InstanceAction(START)`
- **Stop** — `InstanceAction(SOFTSTOP)` (graceful shutdown; 강제 종료 아님)

둘 다 실행 전 인스턴스 이름 타이핑 확인이 필요하고, 실행 후 자동으로 목록을 새로고침한다. 실패 시(예: "이미 상태 변경 중" 409 Conflict) 상태줄에 원본 에러 메시지가 그대로 표시된다.

## Bastion SSH (`s`, `--write` 필요) — ⚠️ 실사용 미검증

Instance 선택 후 `s`를 누르면:
1. 현재 컴파트먼트의 Bastion을 조회 — 없으면 상태줄에 안내만 뜨고 끝, 1개면 자동 선택, 여러 개면 고르는 화면이 뜬다 (여기까지는 실제 라이브 API로 확인됨).
2. 접속할 OS 계정명을 물어본다 (기본값 `opc`).
3. `~/.ssh/id_ed25519` → `id_rsa` → `id_ecdsa` 순으로 로컬 키를 찾아 Bastion 세션을 생성하고, `ACTIVE`가 될 때까지 최대 90초 기다린다.
4. 세션이 활성화되면 터미널을 그대로 넘겨 실제 `ssh` 명령을 실행한다 — 접속이 끝나면 자동으로 toci 화면으로 복귀한다.

**주의**: 3~4단계는 테넌시 Bastion 쿼터가 가득 차 있어 실제 세션으로 검증하지 못했다. `CreateSession` 요청 필드, 응답 안의 SSH 명령 포맷(`SshMetadata["command"]`의 `<privateKey>` 치환), 실제 접속 성공 여부는 모두 코드 리뷰만 거친 상태다. 처음 써볼 때는 실패할 경우를 염두에 두고, 문제가 있으면 `docs/PROGRESS.md`의 "Bastion 세션 → SSH" 절부터 확인할 것.

## 알려진 제약

- 컴파트먼트 이동은 지연(lazy) 방식이라, 한 번에 이름으로 아무 컴파트먼트나 점프하는 기능은 없다 — 트리를 따라 `Enter`로 내려가야 한다.
- Bastion SSH는 코드는 있지만 실제 세션으로 검증되지 않았다 (바로 위 절, `docs/PROGRESS.md` 참고).
