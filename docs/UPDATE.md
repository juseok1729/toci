# toci — 작업 기록

이번 세션에서 추가한 변경사항. 배경/이유가 코드만 봐서는 안 드러나는 결정 위주로 기록.

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
