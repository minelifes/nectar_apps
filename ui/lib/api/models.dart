// Models for the panel API (api/openapi.yaml). Hand-written for now: only the fields the UI uses.

int _int(Object? v) => v is num ? v.toInt() : int.tryParse('$v') ?? 0;
String _str(Object? v) => v is String ? v : (v == null ? '' : '$v');
bool _bool(Object? v) => v == true;
DateTime? _time(Object? v) => v is String ? DateTime.tryParse(v)?.toLocal() : null;
Map<String, dynamic> _map(Object? v) => v is Map ? Map<String, dynamic>.from(v) : const {};
List<dynamic> _list(Object? v) => v is List ? v : const [];

class VersionInfo {
  const VersionInfo({required this.version, required this.apiVersion, this.commit = '', this.minClientVersion = ''});

  final String version;
  final int apiVersion;
  final String commit;
  final String minClientVersion;

  factory VersionInfo.fromJson(Object? j) {
    final m = _map(j);
    return VersionInfo(
      version: _str(m['version']),
      apiVersion: _int(m['api_version']),
      commit: _str(m['commit']),
      minClientVersion: _str(m['min_client_version']),
    );
  }
}

class User {
  const User({required this.id, required this.email, required this.name, required this.isOwner});

  final int id;
  final String email;
  final String name;
  final bool isOwner;

  String get initials {
    final parts = (name.trim().isEmpty ? email : name).trim().split(RegExp(r'[\s@._-]+')).where((p) => p.isNotEmpty);
    return parts.take(2).map((p) => p[0].toUpperCase()).join();
  }

  factory User.fromJson(Object? j) {
    final m = _map(j);
    return User(id: _int(m['id']), email: _str(m['email']), name: _str(m['name']), isOwner: _bool(m['is_owner']));
  }
}

/// Effective permissions (`GET /api/me/permissions`). The UI hides what the user cannot do; the API enforces it.
class Permissions {
  const Permissions({required this.owner, required this.global, required this.projects});

  final bool owner;
  final Set<String> global;
  final Map<int, Set<String>> projects;

  static const none = Permissions(owner: false, global: {}, projects: {});

  /// True if the permission is granted globally or in at least one project.
  bool canAnywhere(String permission) =>
      owner || global.contains(permission) || projects.values.any((p) => p.contains(permission));

  bool can(String permission, {int? projectId}) {
    if (owner || global.contains(permission)) return true;
    if (projectId != null) return projects[projectId]?.contains(permission) ?? false;
    return false;
  }

  factory Permissions.fromJson(Object? j) {
    final m = _map(j);
    final projects = <int, Set<String>>{};
    for (final e in _map(m['projects']).entries) {
      projects[int.tryParse(e.key) ?? -1] = _list(e.value).map(_str).toSet();
    }
    return Permissions(owner: _bool(m['owner']), global: _list(m['global']).map(_str).toSet(), projects: projects);
  }
}

/// CPU/memory amounts reported for nodes.
class NodeResources {
  const NodeResources({required this.cpuMillicores, required this.memoryBytes, this.storageBytes = 0, this.pods = 0});

  final int cpuMillicores;
  final int memoryBytes;
  final int storageBytes;
  final int pods;

  factory NodeResources.fromJson(Object? j) {
    final m = _map(j);
    return NodeResources(
      cpuMillicores: _int(m['cpu_millicores']),
      memoryBytes: _int(m['memory_bytes']),
      storageBytes: _int(m['ephemeral_storage_bytes']),
      pods: _int(m['pods']),
    );
  }
}

class NodeInfo {
  const NodeInfo({
    required this.name,
    required this.ready,
    required this.roles,
    required this.internalIp,
    required this.externalIp,
    required this.os,
    required this.arch,
    required this.kubelet,
    required this.capacity,
    required this.allocatable,
    this.usage,
    this.unschedulable = false,
    this.edge = false,
  });

  /// Cordoned (no new pods); edge = runs nectar-gateway (DNS points here).
  final bool unschedulable;
  final bool edge;

  final String name;
  final bool ready;
  final List<String> roles;
  final String internalIp;
  final String externalIp;
  final String os;
  final String arch;
  final String kubelet;
  final NodeResources capacity;
  final NodeResources allocatable;

  /// null when metrics-server is not available.
  final NodeResources? usage;

  double? get cpuRatio => usage == null || allocatable.cpuMillicores == 0 ? null : usage!.cpuMillicores / allocatable.cpuMillicores;
  double? get memoryRatio => usage == null || allocatable.memoryBytes == 0 ? null : usage!.memoryBytes / allocatable.memoryBytes;

  factory NodeInfo.fromJson(Object? j) {
    final m = _map(j);
    return NodeInfo(
      name: _str(m['name']),
      ready: _bool(m['ready']),
      roles: _list(m['roles']).map(_str).toList(),
      internalIp: _str(m['internal_ip']),
      externalIp: _str(m['external_ip']),
      os: _str(m['os_image']),
      arch: _str(m['arch']),
      kubelet: _str(m['kubelet_version']),
      capacity: NodeResources.fromJson(m['capacity']),
      allocatable: NodeResources.fromJson(m['allocatable']),
      usage: m['usage'] == null ? null : NodeResources.fromJson(m['usage']),
      unschedulable: _bool(m['unschedulable']),
      edge: _bool(m['edge']),
    );
  }
}

class Quota {
  const Quota({this.cpuMillicores = 0, this.memoryMb = 0, this.storageGb = 0, this.pods = 0, this.apps = 0});

  final int cpuMillicores;
  final int memoryMb;
  final int storageGb;
  final int pods;
  final int apps;

  factory Quota.fromJson(Object? j) {
    final m = _map(j);
    return Quota(
      cpuMillicores: _int(m['cpu_millicores']),
      memoryMb: _int(m['memory_mb']),
      storageGb: _int(m['storage_gb']),
      pods: _int(m['pods']),
      apps: _int(m['apps']),
    );
  }
}

class Project {
  const Project({
    required this.id,
    required this.slug,
    required this.name,
    required this.namespace,
    required this.apps,
    required this.adopted,
    this.quota,
    this.createdAt,
  });

  final int id;
  final String slug;
  final String name;
  final String namespace;
  final int apps;
  final bool adopted;

  /// null = unlimited.
  final Quota? quota;
  final DateTime? createdAt;

  factory Project.fromJson(Object? j) {
    final m = _map(j);
    return Project(
      id: _int(m['id']),
      slug: _str(m['slug']),
      name: _str(m['name']),
      namespace: _str(m['namespace']),
      apps: _int(m['apps']),
      adopted: _bool(m['adopted']),
      quota: m['quota'] == null ? null : Quota.fromJson(m['quota']),
      createdAt: _time(m['created_at']),
    );
  }
}

class App {
  const App({
    required this.id,
    required this.projectId,
    required this.slug,
    required this.name,
    required this.kind,
    required this.runtime,
    required this.runtimeVersion,
    required this.status,
    required this.statusMessage,
    required this.replicas,
    required this.cpuMillicores,
    required this.memoryMb,
    required this.image,
    required this.port,
    required this.healthType,
    required this.healthPath,
    required this.schedule,
    this.config = const {},
    this.updatedAt,
  });

  final int id;
  final int projectId;
  final String slug;
  final String name;

  /// web | worker | cron | task
  final String kind;
  final String runtime;
  final String runtimeVersion;
  final String status;
  final String statusMessage;
  final int replicas;
  final int cpuMillicores;
  final int memoryMb;
  final String image;
  final int port;
  final String healthType;
  final String healthPath;
  final String schedule;

  /// The full config as the API returns it (the settings editor works on a copy).
  final Map<String, dynamic> config;
  final DateTime? updatedAt;

  int get resumeReplicas => _int(config['resume_replicas']);
  List<AppVolume> get volumes => _list(config['volumes']).map(AppVolume.fromJson).toList();

  bool get isWeb => kind == 'web';
  bool get hasReplicas => kind == 'web' || kind == 'worker';

  /// Resources requested by all replicas (what counts against a quota).
  int get requestedCpu => cpuMillicores * replicas;
  int get requestedMemoryMb => memoryMb * replicas;

  factory App.fromJson(Object? j) {
    final m = _map(j);
    final cfg = _map(m['config']);
    final res = _map(cfg['resources']);
    return App(
      id: _int(m['id']),
      projectId: _int(m['project_id']),
      slug: _str(m['slug']),
      name: _str(m['name']),
      kind: _str(m['kind']),
      runtime: _str(m['runtime']),
      runtimeVersion: _str(m['runtime_version']),
      status: _str(m['status']),
      statusMessage: _str(m['status_message']),
      replicas: _int(cfg['replicas']),
      cpuMillicores: _int(res['cpu_millicores']),
      memoryMb: _int(res['memory_mb']),
      image: _str(cfg['image']),
      port: _int(cfg['port']),
      healthType: _str(_map(cfg['health'])['type']),
      healthPath: _str(_map(cfg['health'])['path']),
      schedule: _str(cfg['schedule']),
      config: cfg,
      updatedAt: _time(m['updated_at']),
    );
  }
}

class AuditEntry {
  const AuditEntry({required this.id, required this.userEmail, required this.action, required this.target, this.createdAt});

  final int id;
  final String userEmail;
  final String action;
  final String target;
  final DateTime? createdAt;

  factory AuditEntry.fromJson(Object? j) {
    final m = _map(j);
    return AuditEntry(
      id: _int(m['id']),
      userEmail: _str(m['user_email']),
      action: _str(m['action']),
      target: _str(m['target']),
      createdAt: _time(m['created_at']),
    );
  }
}

class PanelSettings {
  const PanelSettings({required this.baseDomain, required this.gatewayConfigured, required this.gatewayReachable, required this.routing});

  final String baseDomain;
  final bool gatewayConfigured;
  final bool gatewayReachable;
  final String routing;

  factory PanelSettings.fromJson(Object? j) {
    final m = _map(j);
    final gw = _map(m['gateway']);
    return PanelSettings(
      baseDomain: _str(m['base_domain']),
      gatewayConfigured: _bool(gw['configured']),
      gatewayReachable: _bool(gw['reachable']),
      routing: _str(gw['routing']),
    );
  }
}

class RuntimeVersion {
  const RuntimeVersion({required this.id, required this.label, required this.image, required this.isDefault});

  final String id;
  final String label;
  final String image;
  final bool isDefault;

  factory RuntimeVersion.fromJson(Object? j) {
    final m = _map(j);
    return RuntimeVersion(id: _str(m['id']), label: _str(m['label']), image: _str(m['image']), isDefault: _bool(m['default']));
  }
}

/// An app type from the runtime catalog (`GET /api/runtimes`).
class Runtime {
  const Runtime({
    required this.id,
    required this.name,
    required this.description,
    required this.kinds,
    required this.artifactKind,
    required this.accept,
    required this.versions,
    required this.port,
    required this.healthType,
    required this.healthPath,
  });

  final String id;
  final String name;
  final String description;
  final List<String> kinds;

  /// file | archive | none (none = the user gives a container image)
  final String artifactKind;

  /// Accepted file names: ".jar", ".zip", ".tar.gz", "*".
  final List<String> accept;
  final List<RuntimeVersion> versions;
  final int port;
  final String healthType;
  final String healthPath;

  bool get takesImage => artifactKind == 'none';

  RuntimeVersion? get defaultVersion =>
      versions.isEmpty ? null : versions.firstWhere((v) => v.isDefault, orElse: () => versions.first);

  factory Runtime.fromJson(Object? j) {
    final m = _map(j);
    final art = _map(m['artifact']);
    final health = _map(m['health']);
    return Runtime(
      id: _str(m['id']),
      name: _str(m['name']),
      description: _str(m['description']),
      kinds: _list(m['kinds']).map(_str).toList(),
      artifactKind: _str(art['kind']),
      accept: _list(art['accept']).map(_str).toList(),
      versions: _list(m['versions']).map(RuntimeVersion.fromJson).toList(),
      port: _int(m['port']),
      healthType: _str(health['type']),
      healthPath: _str(health['path']),
    );
  }
}

/// What the panel learned from an uploaded file.
class DetectResult {
  const DetectResult({
    required this.type,
    required this.runtime,
    required this.version,
    required this.port,
    required this.healthPath,
    required this.minJava,
    required this.springBoot,
    required this.springBootVersion,
    required this.elfArch,
    required this.elfLibc,
    required this.archiveMarkers,
    required this.warnings,
  });

  final String type;
  final String runtime;
  final String version;
  final int port;
  final String healthPath;
  final int minJava;
  final bool springBoot;
  final String springBootVersion;
  final String elfArch;
  final String elfLibc;
  final List<String> archiveMarkers;
  final List<String> warnings;

  factory DetectResult.fromJson(Object? j) {
    final m = _map(j);
    final java = _map(m['java']);
    final elf = _map(m['elf']);
    final arc = _map(m['archive']);
    return DetectResult(
      type: _str(m['type']),
      runtime: _str(m['runtime']),
      version: _str(m['version']),
      port: _int(m['port']),
      healthPath: _str(m['health_path']),
      minJava: _int(java['min_java']),
      springBoot: _bool(java['spring_boot']),
      springBootVersion: _str(java['spring_boot_version']),
      elfArch: _str(elf['arch']),
      elfLibc: _str(elf['libc']),
      archiveMarkers: _list(arc['markers']).map(_str).toList(),
      warnings: _list(m['warnings']).map(_str).toList(),
    );
  }
}

class Artifact {
  const Artifact({required this.id, required this.filename, required this.size, required this.sha256, required this.detect, this.createdAt});

  final int id;
  final String filename;
  final int size;
  final String sha256;
  final DetectResult detect;
  final DateTime? createdAt;

  factory Artifact.fromJson(Object? j) {
    final m = _map(j);
    return Artifact(
      id: _int(m['id']),
      filename: _str(m['filename']),
      size: _int(m['size']),
      sha256: _str(m['sha256']),
      detect: DetectResult.fromJson(m['detect']),
      createdAt: _time(m['created_at']),
    );
  }
}

class Release {
  const Release({
    required this.id,
    required this.number,
    required this.status,
    required this.message,
    required this.artifactId,
    required this.image,
    this.createdAt,
    this.finishedAt,
  });

  final int id;
  final int number;

  /// pending | deploying | live | failed | superseded
  final String status;
  final String message;
  final int? artifactId;
  final String image;
  final DateTime? createdAt;
  final DateTime? finishedAt;

  bool get inProgress => status == 'pending' || status == 'deploying';

  factory Release.fromJson(Object? j) {
    final m = _map(j);
    return Release(
      id: _int(m['id']),
      number: _int(m['number']),
      status: _str(m['status']),
      message: _str(m['message']),
      artifactId: m['artifact_id'] == null ? null : _int(m['artifact_id']),
      image: _str(m['image']),
      createdAt: _time(m['created_at']),
      finishedAt: _time(m['finished_at']),
    );
  }
}

class Domain {
  const Domain({
    required this.id,
    required this.host,
    required this.pathPrefix,
    required this.tls,
    required this.isDefault,
    required this.status,
    required this.statusMessage,
    required this.kind,
    required this.redirectTo,
    this.certNotAfter,
    this.forceHttps = false,
    this.rateLimit = 0,
    this.maxBodyMb = 0,
    this.timeoutSeconds = 0,
  });

  final bool forceHttps;
  final int rateLimit; // requests/s per client IP, 0 = unlimited
  final int maxBodyMb;
  final int timeoutSeconds;

  final int id;
  final String host;
  final String pathPrefix;
  final bool tls;
  final bool isDefault;

  /// pending | active | waiting | error
  final String status;
  final String statusMessage;

  /// app | redirect
  final String kind;
  final String redirectTo;
  final DateTime? certNotAfter;

  String get url => '${tls ? 'https' : 'http'}://$host${pathPrefix == '/' || pathPrefix.isEmpty ? '' : pathPrefix}';

  factory Domain.fromJson(Object? j) {
    final m = _map(j);
    return Domain(
      id: _int(m['id']),
      host: _str(m['host']),
      pathPrefix: _str(m['path_prefix']),
      tls: _bool(m['tls']),
      isDefault: _bool(m['is_default']),
      status: _str(m['status']),
      statusMessage: _str(m['status_message']),
      kind: m['kind'] == null ? 'app' : _str(m['kind']),
      redirectTo: _str(m['redirect_to']),
      certNotAfter: _time(m['cert_not_after']),
      forceHttps: _bool(m['force_https']),
      rateLimit: _int(m['rate_limit']),
      maxBodyMb: _int(m['max_body_mb']),
      timeoutSeconds: _int(m['timeout_seconds']),
    );
  }
}

class PodInfo {
  const PodInfo({
    required this.name,
    required this.phase,
    required this.ready,
    required this.restarts,
    required this.release,
    required this.role,
    required this.node,
    required this.problem,
    this.startedAt,
  });

  final String name;
  final String phase;
  final bool ready;
  final int restarts;
  final String release;
  final String role;
  final String node;
  final String problem;
  final DateTime? startedAt;

  factory PodInfo.fromJson(Object? j) {
    final m = _map(j);
    return PodInfo(
      name: _str(m['name']),
      phase: _str(m['phase']),
      ready: _bool(m['ready']),
      restarts: _int(m['restarts']),
      release: _str(m['release']),
      role: _str(m['role']),
      node: _str(m['node']),
      problem: _str(m['problem']),
      startedAt: _time(m['started_at']),
    );
  }
}

/// `GET /api/apps/{id}/status`: stored status plus the live state in Kubernetes.
class AppLiveStatus {
  const AppLiveStatus({
    required this.status,
    required this.message,
    required this.replicas,
    required this.ready,
    required this.pods,
    required this.nodePorts,
    this.pending = const [],
  });

  /// What changed since the live release and needs a deploy: settings | image | environment.
  final List<String> pending;
  final String status;
  final String message;
  final int replicas;
  final int ready;
  final List<PodInfo> pods;

  /// (port, protocol, nodePort)
  final List<(int, String, int)> nodePorts;

  factory AppLiveStatus.fromJson(Object? j) {
    final m = _map(j);
    final live = _map(m['live']);
    return AppLiveStatus(
      status: _str(m['status']),
      message: _str(m['message']),
      replicas: _int(live['replicas']),
      ready: _int(live['ready']),
      pending: _list(m['pending']).map(_str).toList(),
      pods: _list(live['pods']).map(PodInfo.fromJson).toList(),
      nodePorts: _list(live['node_ports']).map((e) {
        final n = _map(e);
        return (_int(n['port']), _str(n['protocol']), _int(n['node_port']));
      }).toList(),
    );
  }
}

class EnvVar {
  EnvVar({required this.key, this.value, this.secret = false});

  String key;

  /// null for a secret whose value is kept on the server.
  String? value;
  bool secret;

  Map<String, Object?> toJson() => {'key': key, if (value != null) 'value': value, 'secret': secret};

  factory EnvVar.fromJson(Object? j) {
    final m = _map(j);
    return EnvVar(key: _str(m['key']), value: m['value'] == null ? null : _str(m['value']), secret: _bool(m['secret']));
  }
}

double _dbl(Object? v) => v is num ? v.toDouble() : double.tryParse('$v') ?? 0;

/// One averaged point of resource usage history.
class MetricPoint {
  const MetricPoint({required this.t, required this.cpuM, required this.memB, this.diskUsed = 0, this.diskTotal = 0, this.rxBps = 0, this.txBps = 0});

  final DateTime t;
  final int cpuM;
  final int memB;
  final int diskUsed;
  final int diskTotal;
  final int rxBps;
  final int txBps;

  factory MetricPoint.fromJson(Object? j) {
    final m = _map(j);
    return MetricPoint(
      t: _time(m['t']) ?? DateTime.now(),
      cpuM: _int(m['cpu_m']),
      memB: _int(m['mem_b']),
      diskUsed: _int(m['disk_used']),
      diskTotal: _int(m['disk_total']),
      rxBps: _int(m['rx_bps']),
      txBps: _int(m['tx_bps']),
    );
  }
}

class NodeHistory {
  const NodeHistory({required this.name, required this.cpuAllocM, required this.memAllocB, required this.points});

  final String name;
  final int cpuAllocM;
  final int memAllocB;
  final List<MetricPoint> points;

  factory NodeHistory.fromJson(Object? j) {
    final m = _map(j);
    return NodeHistory(
      name: _str(m['name']),
      cpuAllocM: _int(m['cpu_alloc_m']),
      memAllocB: _int(m['mem_alloc_b']),
      points: _list(m['points']).map(MetricPoint.fromJson).toList(),
    );
  }
}

/// `GET /api/apps/{id}/usage`: usage history plus what the app requests (all replicas).
class AppUsage {
  const AppUsage({required this.points, required this.requestCpuM, required this.requestMemB, required this.replicas});

  final List<MetricPoint> points;
  final int requestCpuM;
  final int requestMemB;
  final int replicas;

  factory AppUsage.fromJson(Object? j) {
    final m = _map(j);
    return AppUsage(
      points: _list(m['points']).map(MetricPoint.fromJson).toList(),
      requestCpuM: _int(m['request_cpu_m']),
      requestMemB: _int(m['request_mem_b']),
      replicas: _int(m['replicas']),
    );
  }
}

/// A Kubernetes event.
class KubeEvent {
  const KubeEvent({
    required this.type,
    required this.reason,
    required this.message,
    required this.object,
    required this.namespace,
    required this.count,
    this.last,
  });

  final String type;
  final String reason;
  final String message;
  final String object;
  final String namespace;
  final int count;
  final DateTime? last;

  bool get isWarning => type == 'Warning';

  factory KubeEvent.fromJson(Object? j) {
    final m = _map(j);
    return KubeEvent(
      type: _str(m['type']),
      reason: _str(m['reason']),
      message: _str(m['message']),
      object: _str(m['object']),
      namespace: _str(m['namespace']),
      count: _int(m['count']),
      last: _time(m['last']),
    );
  }
}

class LogLine {
  const LogLine({required this.pod, required this.line, this.t});

  final String pod;
  final String line;
  final DateTime? t;

  factory LogLine.fromJson(Object? j) {
    final m = _map(j);
    return LogLine(pod: _str(m['pod']), line: _str(m['line']), t: _time(m['t']));
  }
}

class PodBrief {
  const PodBrief({required this.name, required this.phase, required this.restarts, required this.role});

  final String name;
  final String phase;
  final int restarts;
  final String role;

  factory PodBrief.fromJson(Object? j) {
    final m = _map(j);
    return PodBrief(name: _str(m['name']), phase: _str(m['phase']), restarts: _int(m['restarts']), role: _str(m['role']));
  }
}

/// Request statistics of one domain from the gateway (last N minutes).
class TrafficSummary {
  const TrafficSummary({
    required this.domain,
    required this.requests,
    required this.rps,
    required this.errorRate,
    required this.p50,
    required this.p95,
    required this.p99,
    required this.status,
    required this.series,
  });

  final String domain;
  final int requests;
  final double rps;
  final double errorRate;
  final int p50;
  final int p95;
  final int p99;
  final Map<String, int> status;

  /// per minute: (time, requests, 5xx errors, p95 ms)
  final List<(DateTime, int, int, int)> series;

  factory TrafficSummary.fromJson(Object? j) {
    final m = _map(j);
    return TrafficSummary(
      domain: _str(m['domain']),
      requests: _int(m['requests']),
      rps: _dbl(m['rps']),
      errorRate: _dbl(m['error_rate']),
      p50: _int(m['p50_ms']),
      p95: _int(m['p95_ms']),
      p99: _int(m['p99_ms']),
      status: {for (final e in _map(m['status']).entries) e.key: _int(e.value)},
      series: _list(m['series']).map((e) {
        final p = _map(e);
        return (_time(p['time']) ?? DateTime.now(), _int(p['requests']), _int(p['errors']), _int(p['p95_ms']));
      }).toList(),
    );
  }
}

class AccessLogEntry {
  const AccessLogEntry({
    required this.time,
    required this.ip,
    required this.method,
    required this.host,
    required this.path,
    required this.status,
    required this.ms,
    required this.bytesOut,
  });

  final DateTime? time;
  final String ip;
  final String method;
  final String host;
  final String path;
  final int status;
  final int ms;
  final int bytesOut;

  factory AccessLogEntry.fromJson(Object? j) {
    final m = _map(j);
    return AccessLogEntry(
      time: _time(m['time']),
      ip: _str(m['ip']),
      method: _str(m['method']),
      host: _str(m['host']),
      path: _str(m['path']),
      status: _int(m['status']),
      ms: _int(m['ms']),
      bytesOut: _int(m['bytes_out']),
    );
  }
}

List<T> listOf<T>(Object? json, T Function(Object?) parse) => _list(json).map(parse).toList();

// ------------------------------------------------------------------ alerts --

/// One alert incident.
class AlertIncident {
  const AlertIncident({
    required this.id,
    required this.rule,
    required this.subject,
    required this.severity,
    required this.message,
    required this.state,
    this.key = '',
    this.appId,
    this.startedAt,
    this.resolvedAt,
    this.notified = 0,
  });

  final int id;
  final String rule;
  final String subject;
  final String severity; // warning | critical
  final String message;
  final String state; // firing | resolved
  final String key; // rule:subject-id, e.g. app_down:12
  final int? appId;
  final DateTime? startedAt;
  final DateTime? resolvedAt;
  final int notified;

  bool get firing => state == 'firing';
  bool get critical => severity == 'critical';

  factory AlertIncident.fromJson(Object? j) {
    final m = _map(j);
    return AlertIncident(
      id: _int(m['id']),
      rule: _str(m['rule']),
      subject: _str(m['subject']),
      severity: _str(m['severity']),
      message: _str(m['message']),
      state: _str(m['state']),
      key: _str(m['key']),
      appId: m['app_id'] == null ? null : _int(m['app_id']),
      startedAt: _time(m['started_at']),
      resolvedAt: _time(m['resolved_at']),
      notified: _int(m['notified']),
    );
  }
}

/// A built-in alert rule from the catalog.
class AlertRuleDef {
  const AlertRuleDef({
    required this.kind,
    required this.title,
    required this.description,
    required this.severity,
    required this.unit,
    required this.defaultThreshold,
    required this.defaultFor,
    this.event = false,
  });

  /// Event rules notify once per occurrence (e.g. a finished deploy) instead of opening an incident.
  final bool event;

  final String kind;
  final String title;
  final String description;
  final String severity;
  final String unit; // "%", "days" or "" (no threshold)
  final double defaultThreshold;
  final int defaultFor;

  factory AlertRuleDef.fromJson(Object? j) {
    final m = _map(j);
    return AlertRuleDef(
      kind: _str(m['kind']),
      title: _str(m['title']),
      description: _str(m['description']),
      severity: _str(m['severity']),
      unit: _str(m['unit']),
      defaultThreshold: _dbl(m['default_threshold']),
      defaultFor: _int(m['default_for']),
      event: _bool(m['event']),
    );
  }
}

class AlertRule {
  const AlertRule({required this.kind, required this.enabled, required this.threshold, required this.forSeconds});

  final String kind;
  final bool enabled;
  final double threshold;
  final int forSeconds;

  AlertRule copyWith({bool? enabled, double? threshold, int? forSeconds}) => AlertRule(
        kind: kind,
        enabled: enabled ?? this.enabled,
        threshold: threshold ?? this.threshold,
        forSeconds: forSeconds ?? this.forSeconds,
      );

  Map<String, dynamic> toJson() => {'kind': kind, 'enabled': enabled, 'threshold': threshold, 'for': forSeconds};

  factory AlertRule.fromJson(Object? j) {
    final m = _map(j);
    return AlertRule(kind: _str(m['kind']), enabled: _bool(m['enabled']), threshold: _dbl(m['threshold']), forSeconds: _int(m['for']));
  }
}

class AlertConfig {
  const AlertConfig({required this.catalog, required this.rules, required this.panelUrl, required this.repeatHours});

  final List<AlertRuleDef> catalog;
  final List<AlertRule> rules;
  final String panelUrl;
  final int repeatHours;

  AlertRuleDef? def(String kind) {
    for (final d in catalog) {
      if (d.kind == kind) return d;
    }
    return null;
  }

  factory AlertConfig.fromJson(Object? j) {
    final m = _map(j);
    final s = _map(m['settings']);
    return AlertConfig(
      catalog: _list(m['catalog']).map(AlertRuleDef.fromJson).toList(),
      rules: _list(s['rules']).map(AlertRule.fromJson).toList(),
      panelUrl: _str(s['panel_url']),
      repeatHours: _int(s['repeat_hours']),
    );
  }
}

/// A notification channel; secret config values come back as [maskedSecret].
class AlertChannel {
  const AlertChannel({required this.id, required this.kind, required this.name, required this.enabled, required this.config});

  final int id;
  final String kind; // ntfy | telegram | email | webhook
  final String name;
  final bool enabled;
  final Map<String, dynamic> config;

  String get summary => switch (kind) {
        'ntfy' => '${_str(config['url'])}/${_str(config['topic'])}',
        'telegram' => 'chat ${_str(config['chat_id'])}',
        'email' => _str(config['to']),
        'webhook' => _str(config['url']),
        _ => kind,
      };

  factory AlertChannel.fromJson(Object? j) {
    final m = _map(j);
    return AlertChannel(id: _int(m['id']), kind: _str(m['kind']), name: _str(m['name']), enabled: _bool(m['enabled']), config: _map(m['config']));
  }
}

/// The value the API returns (and accepts back) for an unchanged secret.
const maskedSecret = '••••••••';

// ------------------------------------------------------------ housekeeping --

class NodeDisk {
  const NodeDisk({required this.name, required this.fsUsed, required this.fsTotal, required this.imagesUsed, required this.imagesTotal, required this.error});

  final String name;
  final int fsUsed;
  final int fsTotal;
  final int imagesUsed;
  final int imagesTotal;
  final String error;

  factory NodeDisk.fromJson(Object? j) {
    final m = _map(j);
    return NodeDisk(
      name: _str(m['name']),
      fsUsed: _int(m['fs_used']),
      fsTotal: _int(m['fs_total']),
      imagesUsed: _int(m['images_used']),
      imagesTotal: _int(m['images_total']),
      error: _str(m['error']),
    );
  }
}

class ImageCleanup {
  const ImageCleanup({required this.name, required this.node, required this.status, required this.removed, required this.message, this.started, this.finished});

  final String name;
  final String node;
  final String status; // running | succeeded | failed
  final int removed;
  final String message;
  final DateTime? started;
  final DateTime? finished;

  factory ImageCleanup.fromJson(Object? j) {
    final m = _map(j);
    return ImageCleanup(
      name: _str(m['name']),
      node: _str(m['node']),
      status: _str(m['status']),
      removed: _int(m['removed']),
      message: _str(m['message']),
      started: _time(m['started']),
      finished: _time(m['finished']),
    );
  }
}

class HousekeepingSettings {
  const HousekeepingSettings({
    required this.keepArtifacts,
    required this.auditDays,
    required this.alertDays,
    required this.k3sDataDir,
    this.prepull = false,
    this.prepullImages = const [],
  });

  final int keepArtifacts;
  final int auditDays;
  final int alertDays;
  final String k3sDataDir;
  final bool prepull;
  final List<String> prepullImages;

  Map<String, dynamic> toJson() => {
        'keep_artifacts': keepArtifacts,
        'audit_days': auditDays,
        'alert_days': alertDays,
        'k3s_data_dir': k3sDataDir,
        'prepull': prepull,
        'prepull_images': prepullImages,
      };

  factory HousekeepingSettings.fromJson(Object? j) {
    final m = _map(j);
    return HousekeepingSettings(
      keepArtifacts: _int(m['keep_artifacts']),
      auditDays: _int(m['audit_days']),
      alertDays: _int(m['alert_days']),
      k3sDataDir: _str(m['k3s_data_dir']),
      prepull: m['prepull'] == true,
      prepullImages: _list(m['prepull_images']).map(_str).toList(),
    );
  }
}

class DiskReport {
  const DiskReport({
    required this.nodes,
    required this.volumeTotal,
    required this.volumeFree,
    required this.databaseBytes,
    required this.artifactBytes,
    required this.artifactFiles,
    required this.uploadBytes,
    required this.cleanups,
    required this.settings,
  });

  final List<NodeDisk> nodes;
  final int volumeTotal;
  final int volumeFree;
  final int databaseBytes;
  final int artifactBytes;
  final int artifactFiles;
  final int uploadBytes;
  final List<ImageCleanup> cleanups;
  final HousekeepingSettings settings;

  bool get cleanupRunning => cleanups.any((j) => j.status == 'running');

  factory DiskReport.fromJson(Object? j) {
    final m = _map(j);
    final p = _map(m['panel']);
    return DiskReport(
      nodes: _list(m['nodes']).map(NodeDisk.fromJson).toList(),
      volumeTotal: _int(p['volume_total']),
      volumeFree: _int(p['volume_free']),
      databaseBytes: _int(p['database_bytes']),
      artifactBytes: _int(p['artifact_bytes']),
      artifactFiles: _int(p['artifact_files']),
      uploadBytes: _int(p['upload_bytes']),
      cleanups: _list(m['image_cleanups']).map(ImageCleanup.fromJson).toList(),
      settings: HousekeepingSettings.fromJson(m['settings']),
    );
  }
}

// -------------------------------------------------------------- operations --

/// A persistent volume of an app (settings).
class AppVolume {
  AppVolume({required this.name, required this.mountPath, required this.sizeMb});

  String name;
  String mountPath;
  int sizeMb;

  Map<String, dynamic> toJson() => {'name': name, 'mount_path': mountPath, 'size_mb': sizeMb};

  factory AppVolume.fromJson(Object? j) {
    final m = _map(j);
    return AppVolume(name: _str(m['name']), mountPath: _str(m['mount_path']), sizeMb: _int(m['size_mb']));
  }
}

/// A volume as it exists in the cluster.
class VolumeInfo {
  const VolumeInfo({
    required this.name,
    required this.claim,
    required this.mountPath,
    required this.requestedMb,
    required this.capacityBytes,
    required this.usedBytes,
    required this.phase,
    required this.inConfig,
    required this.mounted,
  });

  final String name;
  final String claim;
  final String mountPath;
  final int requestedMb;
  final int capacityBytes;
  final int usedBytes;
  final String phase;
  final bool inConfig;
  final bool mounted;

  /// Removed from the settings and no longer mounted: its data waits to be deleted.
  bool get orphaned => !inConfig && !mounted && phase.isNotEmpty;

  factory VolumeInfo.fromJson(Object? j) {
    final m = _map(j);
    return VolumeInfo(
      name: _str(m['name']),
      claim: _str(m['claim']),
      mountPath: _str(m['mount_path']),
      requestedMb: _int(m['requested_mb']),
      capacityBytes: _int(m['capacity_bytes']),
      usedBytes: _int(m['used_bytes']),
      phase: _str(m['phase']),
      inConfig: _bool(m['in_config']),
      mounted: _bool(m['mounted']),
    );
  }
}

/// A config file mounted into the app's containers.
class AppFile {
  AppFile({required this.path, this.content, this.secret = false, this.size = 0});

  String path;

  /// null for a secret file whose content is kept on the server.
  String? content;
  bool secret;
  final int size;

  Map<String, Object?> toJson() => {'path': path, if (content != null) 'content': content, 'secret': secret};

  factory AppFile.fromJson(Object? j) {
    final m = _map(j);
    return AppFile(path: _str(m['path']), content: m['content'] == null ? null : _str(m['content']), secret: _bool(m['secret']), size: _int(m['size']));
  }
}

// ------------------------------------------------------------------ add-ons --

class AddonKindInfo {
  const AddonKindInfo({required this.id, required this.name, required this.port, required this.versions, required this.minMemoryMb});

  final String id;
  final String name;
  final int port;
  final List<(String, bool)> versions; // (id, default)
  final int minMemoryMb;

  String get defaultVersion => versions.where((v) => v.$2).map((v) => v.$1).firstOrNull ?? (versions.isEmpty ? '' : versions.first.$1);

  factory AddonKindInfo.fromJson(Object? j) {
    final m = _map(j);
    return AddonKindInfo(
      id: _str(m['id']),
      name: _str(m['name']),
      port: _int(m['port']),
      minMemoryMb: _int(m['min_memory_mb']),
      versions: _list(m['versions']).map((v) => (_str(_map(v)['id']), _bool(_map(v)['default']))).toList(),
    );
  }
}

/// A database add-on of a project.
class Addon {
  const Addon({
    required this.id,
    required this.projectId,
    required this.slug,
    required this.name,
    required this.kind,
    required this.kindName,
    required this.version,
    required this.sizeMb,
    required this.cpuMillicores,
    required this.memoryMb,
    required this.stopped,
    required this.status,
    required this.statusMessage,
    required this.host,
    required this.port,
    required this.links,
    required this.envNames,
    this.ready,
    this.problem = '',
    this.restarts = 0,
  });

  final int id;
  final int projectId;
  final String slug;
  final String name;
  final String kind;
  final String kindName;
  final String version;
  final int sizeMb;
  final int cpuMillicores;
  final int memoryMb;
  final bool stopped;
  final String status;
  final String statusMessage;
  final String host;
  final int port;

  /// (app id, prefix)
  final List<(int, String)> links;
  final List<String> envNames;

  /// null when the live state is unknown.
  final bool? ready;
  final String problem;
  final int restarts;

  /// running | starting | stopped | failed (for StatusDot/StatusPill)
  String get liveStatus {
    if (stopped) return 'stopped';
    if (status == 'failed') return 'failed';
    if (ready == true) return 'running';
    if (problem.isNotEmpty) return 'failed';
    return 'deploying';
  }

  factory Addon.fromJson(Object? j) {
    final m = _map(j);
    final live = m['live'] == null ? null : _map(m['live']);
    return Addon(
      id: _int(m['id']),
      projectId: _int(m['project_id']),
      slug: _str(m['slug']),
      name: _str(m['name']),
      kind: _str(m['kind']),
      kindName: _str(m['kind_name']),
      version: _str(m['version']),
      sizeMb: _int(m['size_mb']),
      cpuMillicores: _int(m['cpu_millicores']),
      memoryMb: _int(m['memory_mb']),
      stopped: _bool(m['stopped']),
      status: _str(m['status']),
      statusMessage: _str(m['status_message']),
      host: _str(m['host']),
      port: _int(m['port']),
      links: _list(m['links']).map((l) => (_int(_map(l)['app_id']), _str(_map(l)['prefix']))).toList(),
      envNames: _list(m['env_names']).map(_str).toList(),
      ready: live == null ? null : _bool(live['ready']),
      problem: live == null ? '' : _str(live['problem']),
      restarts: live == null ? 0 : _int(live['restarts']),
    );
  }
}

/// An add-on linked to an app, with the variables it provides there.
class AppAddonLink {
  const AppAddonLink({required this.addon, required this.prefix, required this.env});

  final Addon addon;
  final String prefix;
  final List<String> env;

  factory AppAddonLink.fromJson(Object? j) {
    final m = _map(j);
    return AppAddonLink(addon: Addon.fromJson(m['addon']), prefix: _str(m['prefix']), env: _list(m['env']).map(_str).toList());
  }
}

// ------------------------------------------------------------------ backups --

class BackupTarget {
  const BackupTarget({required this.id, required this.name, required this.kind, required this.repository, required this.config, required this.plans});

  final int id;
  final String name;
  final String kind; // s3 | sftp | local
  final String repository;
  final Map<String, dynamic> config; // secrets masked
  final int plans;

  factory BackupTarget.fromJson(Object? j) {
    final m = _map(j);
    return BackupTarget(
      id: _int(m['id']),
      name: _str(m['name']),
      kind: _str(m['kind']),
      repository: _str(m['repository']),
      config: _map(m['config']),
      plans: _int(m['plans']),
    );
  }
}

class BackupRun {
  const BackupRun({
    required this.id,
    required this.planId,
    required this.kind,
    required this.scope,
    required this.subject,
    required this.status,
    required this.message,
    required this.snapshot,
    required this.bytesAdded,
    this.startedAt,
    this.finishedAt,
  });

  final int id;
  final int? planId;
  final String kind; // backup | restore
  final String scope;
  final String subject;
  final String status; // running | succeeded | failed
  final String message;
  final String snapshot;
  final int bytesAdded;
  final DateTime? startedAt;
  final DateTime? finishedAt;

  factory BackupRun.fromJson(Object? j) {
    final m = _map(j);
    return BackupRun(
      id: _int(m['id']),
      planId: m['plan_id'] == null ? null : _int(m['plan_id']),
      kind: _str(m['kind']),
      scope: _str(m['scope']),
      subject: _str(m['subject']),
      status: _str(m['status']),
      message: _str(m['message']),
      snapshot: _str(m['snapshot']),
      bytesAdded: _int(m['bytes_added']),
      startedAt: _time(m['started_at']),
      finishedAt: _time(m['finished_at']),
    );
  }
}

class BackupPlan {
  const BackupPlan({
    required this.id,
    required this.targetId,
    required this.targetName,
    required this.scope,
    required this.subjectId,
    required this.subject,
    required this.schedule,
    required this.keepDaily,
    required this.keepWeekly,
    required this.keepMonthly,
    required this.enabled,
    this.nextRun,
    this.lastRun,
  });

  final int id;
  final int targetId;
  final String targetName;
  final String scope; // app | addon | panel
  final int subjectId;
  final String subject;
  final String schedule;
  final int keepDaily;
  final int keepWeekly;
  final int keepMonthly;
  final bool enabled;
  final DateTime? nextRun;
  final BackupRun? lastRun;

  factory BackupPlan.fromJson(Object? j) {
    final m = _map(j);
    return BackupPlan(
      id: _int(m['id']),
      targetId: _int(m['target_id']),
      targetName: _str(m['target_name']),
      scope: _str(m['scope']),
      subjectId: _int(m['subject_id']),
      subject: _str(m['subject']),
      schedule: _str(m['schedule']),
      keepDaily: _int(m['keep_daily']),
      keepWeekly: _int(m['keep_weekly']),
      keepMonthly: _int(m['keep_monthly']),
      enabled: _bool(m['enabled']),
      nextRun: _time(m['next_run']),
      lastRun: m['last_run'] == null ? null : BackupRun.fromJson(m['last_run']),
    );
  }
}

class BackupSnapshot {
  const BackupSnapshot({required this.id, required this.shortId, required this.paths, this.time, this.dataAdded = 0});

  final String id;
  final String shortId;
  final List<String> paths;
  final DateTime? time;
  final int dataAdded;

  factory BackupSnapshot.fromJson(Object? j) {
    final m = _map(j);
    return BackupSnapshot(
      id: _str(m['id']),
      shortId: _str(m['short_id']),
      paths: _list(m['paths']).map(_str).toList(),
      time: _time(m['time']),
      dataAdded: _int(_map(m['summary'])['data_added']),
    );
  }
}

// ----------------------------------------------------------------- security --

class UserSession {
  const UserSession({required this.id, required this.kind, required this.userAgent, required this.ip, required this.current, this.createdAt, this.lastSeenAt});

  final int id;
  final String kind; // web | mobile
  final String userAgent;
  final String ip;
  final bool current;
  final DateTime? createdAt;
  final DateTime? lastSeenAt;

  /// A short device description from the user agent.
  String get device {
    final ua = userAgent;
    String os = '';
    for (final (needle, name) in const [('iPhone', 'iPhone'), ('iPad', 'iPad'), ('Android', 'Android'), ('Mac OS', 'macOS'), ('Windows', 'Windows'), ('Linux', 'Linux')]) {
      if (ua.contains(needle)) {
        os = name;
        break;
      }
    }
    String app = '';
    for (final (needle, name) in const [('Dart', 'Nectar app'), ('Edg/', 'Edge'), ('Chrome/', 'Chrome'), ('Firefox/', 'Firefox'), ('Safari/', 'Safari')]) {
      if (ua.contains(needle)) {
        app = name;
        break;
      }
    }
    final parts = [app, os].where((p) => p.isNotEmpty).toList();
    return parts.isEmpty ? (kind == 'mobile' ? 'App' : 'Browser') : parts.join(' on ');
  }

  factory UserSession.fromJson(Object? j) {
    final m = _map(j);
    return UserSession(
      id: _int(m['id']),
      kind: _str(m['kind']),
      userAgent: _str(m['user_agent']),
      ip: _str(m['ip']),
      current: _bool(m['current']),
      createdAt: _time(m['created_at']),
      lastSeenAt: _time(m['last_seen_at']),
    );
  }
}

// ----------------------------------------------------------------- updates --

class ReleaseInfo {
  const ReleaseInfo({
    required this.component,
    required this.version,
    required this.channel,
    required this.kind,
    required this.security,
    required this.breaking,
    required this.migrations,
    required this.notes,
    this.publishedAt,
  });

  final String component;
  final String version;
  final String channel;
  final String kind; // patch | minor | major
  final bool security;
  final bool breaking;
  final bool migrations;
  final String notes;
  final DateTime? publishedAt;

  factory ReleaseInfo.fromJson(Object? j) {
    final m = _map(j);
    return ReleaseInfo(
      component: _str(m['component']),
      version: _str(m['version']),
      channel: _str(m['channel']),
      kind: _str(m['kind']),
      security: _bool(m['security']),
      breaking: _bool(m['breaking']),
      migrations: _bool(m['migrations']),
      notes: _str(m['notes']),
      publishedAt: _time(m['published_at']),
    );
  }
}

class K3sNodeVersion {
  const K3sNodeVersion({required this.name, required this.version, required this.controlPlane, required this.unschedulable});

  final String name;
  final String version;
  final bool controlPlane;
  final bool unschedulable;

  factory K3sNodeVersion.fromJson(Object? j) {
    final m = _map(j);
    return K3sNodeVersion(
      name: _str(m['name']),
      version: _str(m['version']),
      controlPlane: _bool(m['control_plane']),
      unschedulable: _bool(m['unschedulable']),
    );
  }
}

/// What the last check found for the panel, the gateway or k3s.
class ComponentUpdate {
  const ComponentUpdate({
    required this.installed,
    required this.available,
    required this.waiting,
    required this.error,
    required this.source,
    this.latest,
    this.auto,
    this.checkedAt,
    this.controller = false,
    this.nodes = const [],
  });

  final String installed;
  final List<ReleaseInfo> available;
  final ReleaseInfo? latest;
  final ReleaseInfo? auto;
  final String waiting;
  final String error;
  final String source;
  final DateTime? checkedAt;
  // k3s
  final bool controller;
  final List<K3sNodeVersion> nodes;

  bool get hasUpdate => available.isNotEmpty;

  static const empty = ComponentUpdate(installed: '', available: [], waiting: '', error: '', source: '');

  factory ComponentUpdate.fromJson(Object? j) {
    final m = _map(j);
    final extra = _map(m['extra']);
    return ComponentUpdate(
      installed: _str(_map(m['installed'])['version']),
      available: listOf(m['available'], ReleaseInfo.fromJson),
      latest: m['latest'] == null ? null : ReleaseInfo.fromJson(m['latest']),
      auto: m['auto'] == null ? null : ReleaseInfo.fromJson(m['auto']),
      waiting: _str(m['waiting']),
      error: _str(m['error']),
      source: _str(m['source']),
      checkedAt: _time(m['checked_at']),
      controller: _bool(extra['controller']),
      nodes: listOf(extra['nodes'], K3sNodeVersion.fromJson),
    );
  }
}

class UpdateStep {
  const UpdateStep({required this.component, required this.from, required this.to, required this.state, required this.message, required this.migrations});

  final String component;
  final String from;
  final String to;
  final String state; // pending | running | done | skipped | failed | rolled_back
  final String message;
  final bool migrations;

  factory UpdateStep.fromJson(Object? j) {
    final m = _map(j);
    return UpdateStep(
      component: _str(m['component']),
      from: _str(m['from']),
      to: _str(m['to']),
      state: _str(m['state']),
      message: _str(m['message']),
      migrations: _bool(m['migrations']),
    );
  }
}

class UpdateRunInfo {
  const UpdateRunInfo({
    required this.id,
    required this.trigger,
    required this.state,
    required this.summary,
    required this.error,
    required this.steps,
    required this.log,
    this.startedAt,
    this.finishedAt,
  });

  final int id;
  final String trigger; // manual | auto | rollback
  final String state; // pending | running | succeeded | failed | rolled_back | rollback_failed
  final String summary;
  final String error;
  final List<UpdateStep> steps;
  final List<(DateTime?, String)> log;
  final DateTime? startedAt;
  final DateTime? finishedAt;

  bool get active => finishedAt == null;

  factory UpdateRunInfo.fromJson(Object? j) {
    final m = _map(j);
    final status = _map(m['status']);
    var steps = listOf(status['steps'], UpdateStep.fromJson);
    if (steps.isEmpty) steps = listOf(_map(m['plan'])['steps'], UpdateStep.fromJson);
    return UpdateRunInfo(
      id: _int(m['id']),
      trigger: _str(m['trigger']),
      state: _str(m['state']),
      summary: _str(m['summary']),
      error: _str(m['error']),
      steps: steps,
      log: [for (final e in _list(status['log'])) (_time(_map(e)['t']), _str(_map(e)['m']))],
      startedAt: _time(m['started_at']),
      finishedAt: _time(m['finished_at']),
    );
  }
}

class UpdatesOverview {
  const UpdatesOverview({
    required this.settings,
    required this.components,
    required this.bad,
    required this.runs,
    required this.windowOpen,
    this.active,
    this.checkedAt,
    this.nextCheck,
  });

  final Map<String, dynamic> settings; // raw, edited by the settings card
  final Map<String, ComponentUpdate> components;
  final Map<String, String> bad;
  final List<UpdateRunInfo> runs;
  final UpdateRunInfo? active;
  final DateTime? checkedAt;
  final DateTime? nextCheck;
  final bool windowOpen;

  ComponentUpdate component(String c) => components[c] ?? ComponentUpdate.empty;

  factory UpdatesOverview.fromJson(Object? j) {
    final m = _map(j);
    final state = _map(m['state']);
    return UpdatesOverview(
      settings: _map(m['settings']),
      components: {for (final e in _map(state['components']).entries) e.key: ComponentUpdate.fromJson(e.value)},
      bad: {for (final e in _map(m['bad']).entries) e.key: _str(e.value)},
      runs: listOf(m['runs'], UpdateRunInfo.fromJson),
      active: m['active'] == null ? null : UpdateRunInfo.fromJson(m['active']),
      checkedAt: _time(state['checked_at']),
      nextCheck: _time(m['next_check']),
      windowOpen: _bool(m['window_open']),
    );
  }
}

// ------------------------------------------------------- users, roles, tokens --

class AdminUser {
  const AdminUser({
    required this.id,
    required this.email,
    required this.name,
    required this.isOwner,
    required this.disabled,
    required this.mfaEnabled,
    this.createdAt,
  });

  final int id;
  final String email;
  final String name;
  final bool isOwner;
  final bool disabled;
  final bool mfaEnabled;
  final DateTime? createdAt;

  String get label => name.isEmpty ? email : name;

  factory AdminUser.fromJson(Object? j) {
    final m = _map(j);
    return AdminUser(
      id: _int(m['id']),
      email: _str(m['email']),
      name: _str(m['name']),
      isOwner: _bool(m['is_owner']),
      disabled: _bool(m['disabled']),
      mfaEnabled: _bool(m['mfa_enabled']),
      createdAt: _time(m['created_at']),
    );
  }
}

class RoleInfo {
  const RoleInfo({
    required this.id,
    required this.name,
    required this.description,
    required this.builtinKey,
    required this.permissions,
    required this.bindings,
  });

  final int id;
  final String name;
  final String description;
  final String builtinKey;
  final List<String> permissions;
  final int bindings;

  bool get builtin => builtinKey.isNotEmpty;

  factory RoleInfo.fromJson(Object? j) {
    final m = _map(j);
    return RoleInfo(
      id: _int(m['id']),
      name: _str(m['name']),
      description: _str(m['description']),
      builtinKey: _str(m['builtin_key']),
      permissions: [for (final p in _list(m['permissions'])) _str(p)],
      bindings: _int(m['bindings']),
    );
  }
}

class PermissionDef {
  const PermissionDef({required this.key, required this.description, required this.projectScope});

  final String key;
  final String description;
  final bool projectScope;
}

class PermissionGroup {
  const PermissionGroup({required this.name, required this.permissions});

  final String name;
  final List<PermissionDef> permissions;

  factory PermissionGroup.fromJson(Object? j) {
    final m = _map(j);
    return PermissionGroup(
      name: _str(m['group']),
      permissions: [
        for (final p in _list(m['permissions']))
          PermissionDef(key: _str(_map(p)['key']), description: _str(_map(p)['description']), projectScope: _map(p)['scope'] == 'project'),
      ],
    );
  }
}

class RoleBindingInfo {
  const RoleBindingInfo({required this.roleId, required this.roleName, this.projectId});

  final int roleId;
  final String roleName;
  final int? projectId;

  Map<String, dynamic> toJson() => {'role_id': roleId, 'project_id': projectId};

  factory RoleBindingInfo.fromJson(Object? j) {
    final m = _map(j);
    return RoleBindingInfo(roleId: _int(m['role_id']), roleName: _str(m['role_name']), projectId: m['project_id'] == null ? null : _int(m['project_id']));
  }
}

class ApiTokenInfo {
  const ApiTokenInfo({
    required this.id,
    required this.name,
    required this.userEmail,
    required this.roleName,
    required this.projectName,
    this.roleId,
    this.projectId,
    this.createdAt,
    this.lastUsedAt,
    this.expiresAt,
  });

  final int id;
  final String name;
  final String userEmail;
  final int? roleId;
  final String roleName;
  final int? projectId;
  final String projectName;
  final DateTime? createdAt;
  final DateTime? lastUsedAt;
  final DateTime? expiresAt;

  bool get expired => expiresAt != null && expiresAt!.isBefore(DateTime.now());

  factory ApiTokenInfo.fromJson(Object? j) {
    final m = _map(j);
    return ApiTokenInfo(
      id: _int(m['id']),
      name: _str(m['name']),
      userEmail: _str(m['user_email']),
      roleId: m['role_id'] == null ? null : _int(m['role_id']),
      roleName: _str(m['role_name']),
      projectId: m['project_id'] == null ? null : _int(m['project_id']),
      projectName: _str(m['project_name']),
      createdAt: _time(m['created_at']),
      lastUsedAt: _time(m['last_used_at']),
      expiresAt: _time(m['expires_at']),
    );
  }
}

class ProjectMember {
  const ProjectMember({
    required this.userId,
    required this.name,
    required this.email,
    required this.isOwner,
    required this.disabled,
    required this.roles,
    required this.globalRoles,
  });

  final int userId;
  final String name;
  final String email;
  final bool isOwner;
  final bool disabled;
  final List<RoleBindingInfo> roles; // in this project
  final List<RoleBindingInfo> globalRoles;

  factory ProjectMember.fromJson(Object? j) {
    final m = _map(j);
    return ProjectMember(
      userId: _int(m['user_id']),
      name: _str(m['name']),
      email: _str(m['email']),
      isOwner: _bool(m['is_owner']),
      disabled: _bool(m['disabled']),
      roles: listOf(m['roles'], RoleBindingInfo.fromJson),
      globalRoles: listOf(m['global_roles'], RoleBindingInfo.fromJson),
    );
  }
}

// ------------------------------------------------------------------ gateway --

class GatewayRoute {
  const GatewayRoute({
    required this.id,
    required this.domain,
    required this.pathPrefix,
    required this.mode,
    required this.target,
    required this.rateLimit,
    required this.acme,
    required this.forceSsl,
    required this.certSource,
    required this.managedBy,
    required this.maintenance,
    required this.hcEnabled,
    this.certNotAfter,
    this.coveredBy = '',
    this.coveredUntil,
  });

  final int id;
  final String domain;
  final String pathPrefix;
  final String mode;
  final String target;
  final int rateLimit;
  final bool acme;
  final bool forceSsl;
  final String certSource;
  final String managedBy;
  final bool maintenance;
  final bool hcEnabled;
  final DateTime? certNotAfter;

  /// The wildcard (standalone) certificate that serves this host when the route has none of its own.
  final String coveredBy;
  final DateTime? coveredUntil;

  String get label => pathPrefix.isEmpty || pathPrefix == '/' ? domain : '$domain$pathPrefix';

  factory GatewayRoute.fromJson(Object? j) {
    final m = _map(j);
    final notAfter = _int(m['cert_not_after']);
    final port = _int(m['target_port']);
    return GatewayRoute(
      id: _int(m['id']),
      domain: _str(m['domain']),
      pathPrefix: _str(m['path_prefix']),
      mode: _str(m['mode']),
      target: _str(m['mode']) == 'redirect' ? _str(m['redirect_url']) : '${_str(m['target_ip'])}${port > 0 ? ':$port' : ''}',
      rateLimit: _int(m['rate_limit']),
      acme: _bool(m['acme']),
      forceSsl: _bool(m['force_ssl']),
      certSource: _str(m['cert_source']),
      managedBy: _str(m['managed_by']),
      maintenance: _bool(m['maintenance']),
      hcEnabled: _bool(m['hc_enabled']),
      certNotAfter: notAfter > 0 ? DateTime.fromMillisecondsSinceEpoch(notAfter * 1000) : null,
      coveredBy: _str(m['covered_by']),
      coveredUntil: _int(m['covered_until']) > 0 ? DateTime.fromMillisecondsSinceEpoch(_int(m['covered_until']) * 1000) : null,
    );
  }
}

/// A standalone gateway certificate, e.g. *.apps.example.com over DNS-01.
class GatewayCertificate {
  const GatewayCertificate({
    required this.id,
    required this.domain,
    required this.names,
    required this.acme,
    required this.status,
    required this.lastError,
    required this.serves,
    required this.managedBy,
    this.notAfter,
  });

  final int id;
  final String domain;
  final List<String> names;
  final bool acme;
  final String status; // pending | valid | expiring | expired | failed
  final String lastError;
  final List<String> serves;
  final String managedBy;
  final DateTime? notAfter;

  factory GatewayCertificate.fromJson(Object? j) {
    final m = _map(j);
    final na = _int(m['not_after']);
    return GatewayCertificate(
      id: _int(m['id']),
      domain: _str(m['domain']),
      names: _list(m['names']).map(_str).toList(),
      acme: _bool(m['acme']),
      status: _str(m['status']),
      lastError: _str(m['last_error']),
      serves: _list(m['serves']).map(_str).toList(),
      managedBy: _str(m['managed_by']),
      notAfter: na > 0 ? DateTime.fromMillisecondsSinceEpoch(na * 1000) : null,
    );
  }
}

/// GET /api/gateway/certificates: what the gateway supports plus its standalone certificates.
class CertificatesInfo {
  const CertificatesInfo({
    required this.supported,
    required this.acme,
    required this.dns,
    required this.dnsProvider,
    required this.baseWildcard,
    required this.certificates,
  });

  final bool supported; // the gateway has the certificates API (≥ 1.2.2)
  final bool acme; // its Let's Encrypt client is on
  final bool dns; // a DNS provider is set: wildcards possible
  final String dnsProvider;
  final String baseWildcard; // "*.<base domain>" or ""
  final List<GatewayCertificate> certificates;

  bool get hasBaseWildcard => baseWildcard.isNotEmpty && certificates.any((c) => c.domain == baseWildcard);

  factory CertificatesInfo.fromJson(Object? j) {
    final m = _map(j);
    return CertificatesInfo(
      supported: _bool(m['supported']),
      acme: _bool(m['acme']),
      dns: _bool(m['dns']),
      dnsProvider: _str(m['dns_provider']),
      baseWildcard: _str(m['base_wildcard']),
      certificates: _list(m['certificates']).map(GatewayCertificate.fromJson).toList(),
    );
  }
}

class RouteHealth {
  const RouteHealth({this.up, required this.lastError, this.lastCheck});

  final bool? up; // null until the first check
  final String lastError;
  final DateTime? lastCheck;

  factory RouteHealth.fromJson(Object? j) {
    final m = _map(j);
    return RouteHealth(up: m['up'] is bool ? m['up'] as bool : null, lastError: _str(m['last_error']), lastCheck: _time(m['last_check']));
  }
}

class BlockedIp {
  const BlockedIp({required this.ip, required this.reason, this.blockedAt, this.expiresAt});

  final String ip;
  final String reason;
  final DateTime? blockedAt;
  final DateTime? expiresAt;

  factory BlockedIp.fromJson(Object? j) {
    final m = _map(j);
    return BlockedIp(ip: _str(m['ip']), reason: _str(m['reason']), blockedAt: _time(m['blocked_at']), expiresAt: _time(m['expires_at']));
  }
}

// ------------------------------------------------------------- Git builds --

class AppSourceInfo {
  const AppSourceInfo({
    required this.repoUrl,
    required this.branch,
    required this.contextDir,
    required this.dockerfile,
    required this.hasToken,
    required this.autoDeploy,
    required this.webhookUrl,
    required this.webhookSecret,
    required this.buildCpu,
    required this.buildMemory,
  });

  final String repoUrl;
  final String branch;
  final String contextDir;
  final String dockerfile;
  final bool hasToken;
  final bool autoDeploy;
  final String webhookUrl;
  final String webhookSecret; // empty without app.edit
  final int buildCpu;
  final int buildMemory;

  factory AppSourceInfo.fromJson(Object? j) {
    final m = _map(j);
    return AppSourceInfo(
      repoUrl: _str(m['repo_url']),
      branch: _str(m['branch']),
      contextDir: _str(m['context_dir']),
      dockerfile: _str(m['dockerfile']),
      hasToken: _bool(m['has_token']),
      autoDeploy: _bool(m['auto_deploy']),
      webhookUrl: _str(m['webhook_url']),
      webhookSecret: _str(m['webhook_secret']),
      buildCpu: _int(m['build_cpu_millicores']),
      buildMemory: _int(m['build_memory_mb']),
    );
  }
}

class BuildInfo {
  const BuildInfo({
    required this.id,
    required this.number,
    required this.trigger,
    required this.branch,
    required this.commitSha,
    required this.commitMessage,
    required this.status,
    required this.image,
    required this.deploy,
    required this.message,
    this.release,
    this.startedAt,
    this.finishedAt,
  });

  final int id;
  final int number;
  final String trigger; // manual | webhook | api
  final String branch;
  final String commitSha;
  final String commitMessage;
  final String status; // running | succeeded | failed | canceled
  final String image;
  final bool deploy;
  final String message;
  final int? release;
  final DateTime? startedAt;
  final DateTime? finishedAt;

  bool get running => status == 'running';
  String get shortSha => commitSha.length > 7 ? commitSha.substring(0, 7) : commitSha;

  factory BuildInfo.fromJson(Object? j) {
    final m = _map(j);
    return BuildInfo(
      id: _int(m['id']),
      number: _int(m['number']),
      trigger: _str(m['trigger']),
      branch: _str(m['branch']),
      commitSha: _str(m['commit_sha']),
      commitMessage: _str(m['commit_message']),
      status: _str(m['status']),
      image: _str(m['image']),
      deploy: _bool(m['deploy']),
      message: _str(m['message']),
      release: m['release'] == null ? null : _int(m['release']),
      startedAt: _time(m['started_at']),
      finishedAt: _time(m['finished_at']),
    );
  }
}

class RegistryLogin {
  const RegistryLogin({required this.id, required this.host, required this.username, this.updatedAt});

  final int id;
  final String host;
  final String username;
  final DateTime? updatedAt;

  factory RegistryLogin.fromJson(Object? j) {
    final m = _map(j);
    return RegistryLogin(id: _int(m['id']), host: _str(m['host']), username: _str(m['username']), updatedAt: _time(m['updated_at']));
  }
}

class RegistrySettings {
  const RegistrySettings({required this.logins, required this.builtinEnabled, required this.builtinHost});

  final List<RegistryLogin> logins;
  final bool builtinEnabled;
  final String builtinHost;

  factory RegistrySettings.fromJson(Object? j) {
    final m = _map(j);
    final b = _map(m['builtin']);
    return RegistrySettings(logins: listOf(m['logins'], RegistryLogin.fromJson), builtinEnabled: _bool(b['enabled']), builtinHost: _str(b['host']));
  }
}
