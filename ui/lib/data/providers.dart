import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/client.dart';
import '../api/models.dart';
import '../api/session.dart';

final nodesProvider = FutureProvider.autoDispose<List<NodeInfo>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/server/nodes'), NodeInfo.fromJson);
});

final projectsProvider = FutureProvider.autoDispose<List<Project>>((ref) async {
  final api = ref.watch(apiProvider);
  final list = listOf(await api.get('/projects'), Project.fromJson);
  list.sort((a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));
  return list;
});

final projectProvider = FutureProvider.autoDispose.family<Project, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return Project.fromJson(await api.get('/projects/$id'));
});

final projectAppsProvider = FutureProvider.autoDispose.family<List<App>, int>((ref, projectId) async {
  final api = ref.watch(apiProvider);
  final list = listOf(await api.get('/projects/$projectId/apps'), App.fromJson);
  list.sort((a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));
  return list;
});

/// Every project with its apps (dashboard). Projects the user cannot list apps for come back empty.
final projectsWithAppsProvider = FutureProvider.autoDispose<List<(Project, List<App>)>>((ref) async {
  final projects = await ref.watch(projectsProvider.future);
  return Future.wait(
    projects.map((p) async {
      try {
        return (p, await ref.watch(projectAppsProvider(p.id).future));
      } on Object {
        return (p, const <App>[]);
      }
    }),
  );
});

/// Latest activity (needs audit.view; null when the user may not see it).
final recentActivityProvider = FutureProvider.autoDispose<List<AuditEntry>?>((ref) async {
  if (!ref.watch(sessionProvider.select((s) => s.can('audit.view')))) return null;
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/audit', query: {'limit': 8}), AuditEntry.fromJson);
});

final panelSettingsProvider = FutureProvider.autoDispose<PanelSettings>((ref) async {
  final api = ref.watch(apiProvider);
  return PanelSettings.fromJson(await api.get('/settings'));
});

final runtimesProvider = FutureProvider.autoDispose<List<Runtime>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/runtimes'), Runtime.fromJson);
});

final appProvider = FutureProvider.autoDispose.family<App, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return App.fromJson(await api.get('/apps/$id'));
});

final appLiveProvider = FutureProvider.autoDispose.family<AppLiveStatus, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return AppLiveStatus.fromJson(await api.get('/apps/$id/status'));
});

final releasesProvider = FutureProvider.autoDispose.family<List<Release>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$id/releases'), Release.fromJson);
});

final domainsProvider = FutureProvider.autoDispose.family<List<Domain>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$id/domains'), Domain.fromJson);
});

final artifactsProvider = FutureProvider.autoDispose.family<List<Artifact>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$id/artifacts'), Artifact.fromJson);
});

/// Invalidates everything shown for one app (after deploy, rollback, settings changes).
void refreshApp(WidgetRef ref, int id) {
  ref.invalidate(appProvider(id));
  ref.invalidate(appLiveProvider(id));
  ref.invalidate(releasesProvider(id));
  ref.invalidate(domainsProvider(id));
  ref.invalidate(artifactsProvider(id));
}

/// First-run onboarding state (owners / settings.manage only; null for everyone else).
final onboardingProvider = FutureProvider.autoDispose<Map<String, dynamic>?>((ref) async {
  if (!ref.watch(sessionProvider.select((s) => s.can('settings.manage')))) return null;
  final api = ref.watch(apiProvider);
  final res = await api.get('/onboarding');
  return res is Map ? Map<String, dynamic>.from(res) : null;
});

final projectActivityProvider = FutureProvider.autoDispose.family<List<AuditEntry>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/projects/$id/activity', query: {'limit': 15}), AuditEntry.fromJson);
});

/// Usage history ranges offered by the charts.
const usageRanges = ['1h', '6h', '24h', '7d', '30d'];

final serverMetricsProvider = FutureProvider.autoDispose.family<List<NodeHistory>, String>((ref, range) async {
  final api = ref.watch(apiProvider);
  final res = await api.get('/server/metrics', query: {'range': range});
  return listOf(res is Map ? res['nodes'] : null, NodeHistory.fromJson);
});

final serverEventsProvider = FutureProvider.autoDispose<List<KubeEvent>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/server/events'), KubeEvent.fromJson);
});

final appUsageProvider = FutureProvider.autoDispose.family<AppUsage, (int, String)>((ref, key) async {
  final api = ref.watch(apiProvider);
  return AppUsage.fromJson(await api.get('/apps/${key.$1}/usage', query: {'range': key.$2}));
});

final appEventsProvider = FutureProvider.autoDispose.family<List<KubeEvent>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$id/events'), KubeEvent.fromJson);
});

/// Request statistics of the app's domains (last hour, with per-minute series). Empty without a gateway.
final appTrafficProvider = FutureProvider.autoDispose.family<List<TrafficSummary>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  final res = await api.get('/apps/$id/metrics', query: {'minutes': 60, 'series': '1'});
  return listOf(res is Map ? res['domains'] : null, TrafficSummary.fromJson);
});

final appAccessLogProvider = FutureProvider.autoDispose.family<List<AccessLogEntry>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$id/access-log', query: {'limit': 50}), AccessLogEntry.fromJson);
});

/// Firing alerts, refreshed every 30 seconds while watched (empty without server.view).
final firingAlertsProvider = FutureProvider.autoDispose<List<AlertIncident>>((ref) async {
  final canView = ref.watch(sessionProvider.select((s) => s.can('server.view')));
  if (!canView) return const [];
  final api = ref.watch(apiProvider);
  final timer = Timer(const Duration(seconds: 30), ref.invalidateSelf);
  ref.onDispose(timer.cancel);
  return listOf(await api.get('/alerts', query: {'state': 'firing'}), AlertIncident.fromJson);
});

final alertHistoryProvider = FutureProvider.autoDispose<List<AlertIncident>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/alerts', query: {'limit': 200}), AlertIncident.fromJson);
});

final appAlertsProvider = FutureProvider.autoDispose.family<List<AlertIncident>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  final timer = Timer(const Duration(seconds: 30), ref.invalidateSelf);
  ref.onDispose(timer.cancel);
  return listOf(await api.get('/apps/$id/alerts', query: {'state': 'firing'}), AlertIncident.fromJson);
});

final alertConfigProvider = FutureProvider.autoDispose<AlertConfig>((ref) async {
  final api = ref.watch(apiProvider);
  return AlertConfig.fromJson(await api.get('/alerts/rules'));
});

final alertChannelsProvider = FutureProvider.autoDispose<List<AlertChannel>>((ref) async {
  final api = ref.watch(apiProvider);
  final res = await api.get('/alerts/channels');
  return listOf(res is Map ? res['channels'] : null, AlertChannel.fromJson);
});

/// Disk usage of nodes and the panel, recent image cleanups and retention settings.
final serverDiskProvider = FutureProvider.autoDispose<DiskReport>((ref) async {
  final api = ref.watch(apiProvider);
  final report = DiskReport.fromJson(await api.get('/server/disk'));
  if (report.cleanupRunning) {
    final timer = Timer(const Duration(seconds: 5), ref.invalidateSelf);
    ref.onDispose(timer.cancel);
  }
  return report;
});

final appEnvProvider = FutureProvider.autoDispose.family<List<EnvVar>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$id/env'), EnvVar.fromJson);
});

final appFilesProvider = FutureProvider.autoDispose.family<List<AppFile>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$id/files'), AppFile.fromJson);
});

final appVolumesProvider = FutureProvider.autoDispose.family<List<VolumeInfo>, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$id/volumes'), VolumeInfo.fromJson);
});

final addonKindsProvider = FutureProvider.autoDispose<List<AddonKindInfo>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/addon-kinds'), AddonKindInfo.fromJson);
});

final projectAddonsProvider = FutureProvider.autoDispose.family<List<Addon>, int>((ref, projectId) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/projects/$projectId/addons'), Addon.fromJson);
});

final addonProvider = FutureProvider.autoDispose.family<Addon, int>((ref, id) async {
  final api = ref.watch(apiProvider);
  return Addon.fromJson(await api.get('/addons/$id'));
});

final appAddonsProvider = FutureProvider.autoDispose.family<List<AppAddonLink>, int>((ref, appId) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/apps/$appId/addons'), AppAddonLink.fromJson);
});

final backupTargetsProvider = FutureProvider.autoDispose<List<BackupTarget>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/backup/targets'), BackupTarget.fromJson);
});

/// Plans, refreshed every 10 seconds while a run is in progress.
final backupPlansProvider = FutureProvider.autoDispose<List<BackupPlan>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/backup/plans'), BackupPlan.fromJson);
});

final backupRunsProvider = FutureProvider.autoDispose<List<BackupRun>>((ref) async {
  final api = ref.watch(apiProvider);
  final runs = listOf(await api.get('/backup/runs', query: {'limit': 100}), BackupRun.fromJson);
  if (runs.any((r) => r.status == 'running')) {
    final timer = Timer(const Duration(seconds: 5), () {
      ref.invalidateSelf();
      ref.invalidate(backupPlansProvider);
    });
    ref.onDispose(timer.cancel);
  }
  return runs;
});

final mfaStatusProvider = FutureProvider.autoDispose<(bool, int)>((ref) async {
  final api = ref.watch(apiProvider);
  final res = await api.get('/me/mfa');
  final m = res is Map ? res : const {};
  return (m['enabled'] == true, (m['recovery_codes_left'] as num?)?.toInt() ?? 0);
});

final mySessionsProvider = FutureProvider.autoDispose<List<UserSession>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/me/sessions'), UserSession.fromJson);
});

final smtpSettingsProvider = FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final api = ref.watch(apiProvider);
  final res = await api.get('/settings/smtp');
  return Map<String, dynamic>.from((res as Map)['settings'] as Map? ?? const {});
});

final accessSettingsProvider = FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final api = ref.watch(apiProvider);
  return Map<String, dynamic>.from(await api.get('/settings/access') as Map);
});

/// Settings → Updates. Refreshed every 3 seconds while an update runs (the panel may restart meanwhile:
/// errors keep the polling going).
final updatesProvider = FutureProvider.autoDispose<UpdatesOverview>((ref) async {
  final api = ref.watch(apiProvider);
  try {
    final o = UpdatesOverview.fromJson(await api.get('/updates'));
    if (o.active != null) {
      final timer = Timer(const Duration(seconds: 3), ref.invalidateSelf);
      ref.onDispose(timer.cancel);
    } else if (ref.read(updateInProgressProvider)) {
      final flag = ref.read(updateInProgressProvider.notifier);
      Future.microtask(() => flag.set(false));
    }
    return o;
  } on Object {
    if (ref.read(updateInProgressProvider)) {
      final timer = Timer(const Duration(seconds: 3), ref.invalidateSelf);
      ref.onDispose(timer.cancel);
    }
    rethrow;
  }
});

/// Set while this browser started or watches an update (keeps polling through the panel's restart).
final updateInProgressProvider = NotifierProvider<UpdateInProgress, bool>(UpdateInProgress.new);

class UpdateInProgress extends Notifier<bool> {
  @override
  bool build() => false;

  void set(bool value) => state = value;
}

/// The server version, checked every 5 minutes (web: "new version, reload"; apps: minimum client version).
final serverVersionProvider = StreamProvider.autoDispose<VersionInfo>((ref) async* {
  final api = ref.watch(apiProvider);
  while (true) {
    try {
      yield VersionInfo.fromJson(await api.get('/version'));
    } on Object {
      // offline for a moment: try again later
    }
    await Future<void>.delayed(const Duration(minutes: 5));
  }
});

// -------------------------------------------------------- users and access --

final usersProvider = FutureProvider.autoDispose<List<AdminUser>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/users'), AdminUser.fromJson);
});

final rolesProvider = FutureProvider.autoDispose<List<RoleInfo>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/roles'), RoleInfo.fromJson);
});

final permissionCatalogProvider = FutureProvider<List<PermissionGroup>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/permissions'), PermissionGroup.fromJson);
});

final userBindingsProvider = FutureProvider.autoDispose.family<List<RoleBindingInfo>, int>((ref, userId) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/users/$userId/bindings'), RoleBindingInfo.fromJson);
});

final myTokensProvider = FutureProvider.autoDispose<List<ApiTokenInfo>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/me/tokens'), ApiTokenInfo.fromJson);
});

final allTokensProvider = FutureProvider.autoDispose<List<ApiTokenInfo>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/tokens'), ApiTokenInfo.fromJson);
});

final projectMembersProvider = FutureProvider.autoDispose.family<List<ProjectMember>, int>((ref, projectId) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/projects/$projectId/members'), ProjectMember.fromJson);
});

/// Active users (id, name, email) for picking project members.
final userDirectoryProvider = FutureProvider.autoDispose<List<AdminUser>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/users/directory'), AdminUser.fromJson);
});

// ------------------------------------------------------------------ gateway --

final gatewayRoutesProvider = FutureProvider.autoDispose<List<GatewayRoute>>((ref) async {
  final api = ref.watch(apiProvider);
  final list = listOf(await api.get('/gateway/routes'), GatewayRoute.fromJson);
  list.sort((a, b) => a.label.compareTo(b.label));
  return list;
});

/// Route id → health, refreshed every 30 seconds.
final gatewayHealthProvider = FutureProvider.autoDispose<Map<int, RouteHealth>>((ref) async {
  final api = ref.watch(apiProvider);
  final res = await api.get('/gateway/health');
  final timer = Timer(const Duration(seconds: 30), ref.invalidateSelf);
  ref.onDispose(timer.cancel);
  return {for (final e in (res is Map ? res : const {}).entries) int.tryParse('${e.key}') ?? 0: RouteHealth.fromJson(e.value)};
});

/// Standalone (wildcard) gateway certificates; polls while one is being issued.
final gatewayCertificatesProvider = FutureProvider.autoDispose<CertificatesInfo>((ref) async {
  final api = ref.watch(apiProvider);
  final info = CertificatesInfo.fromJson(await api.get('/gateway/certificates'));
  if (info.certificates.any((c) => c.status == 'pending')) {
    final t = Timer(const Duration(seconds: 5), ref.invalidateSelf);
    ref.onDispose(t.cancel);
  }
  return info;
});

/// The gateway's runtime settings (fallbacks, Let's Encrypt email, DNS provider, rate-limit blocking).
final gatewaySettingsProvider = FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final api = ref.watch(apiProvider);
  final v = await api.get('/gateway/settings');
  return v is Map ? Map<String, dynamic>.from(v) : <String, dynamic>{};
});

/// Latest requests across every domain of the gateway, filtered by domain, status class (2xx…5xx or a
/// code) and client IP; empty strings mean no filter.
final gatewayAccessLogProvider =
    FutureProvider.autoDispose.family<List<AccessLogEntry>, ({String domain, String status, String ip})>((ref, f) async {
  final api = ref.watch(apiProvider);
  return listOf(
    await api.get('/gateway/access-log', query: {
      'limit': 200,
      if (f.domain.isNotEmpty) 'domain': f.domain,
      if (f.status.isNotEmpty) 'status': f.status,
      if (f.ip.isNotEmpty) 'ip': f.ip,
    }),
    AccessLogEntry.fromJson,
  );
});

/// Every visible domain across projects plus DNS setup info (Domains page).
final domainsOverviewProvider = FutureProvider.autoDispose<DomainsOverview>((ref) async {
  final api = ref.watch(apiProvider);
  return DomainsOverview.fromJson(await api.get('/domains'));
});

/// Storage classes volumes can use.
final storageClassesProvider = FutureProvider.autoDispose<List<StorageClassInfo>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/server/storage-classes'), StorageClassInfo.fromJson);
});

/// Every volume on the server with its usage (Server page).
final serverVolumesProvider = FutureProvider.autoDispose<List<VolumeUse>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/server/volumes'), VolumeUse.fromJson);
});

/// The env groups of a project.
final envGroupsProvider = FutureProvider.autoDispose.family<List<EnvGroupInfo>, int>((ref, projectId) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/projects/$projectId/env-groups'), EnvGroupInfo.fromJson);
});

final blockedIpsProvider = FutureProvider.autoDispose<List<BlockedIp>>((ref) async {
  final api = ref.watch(apiProvider);
  return listOf(await api.get('/gateway/blocked'), BlockedIp.fromJson);
});

// --------------------------------------------------------------- Git builds --

/// The Git source of an app (null when it is not built from Git).
final appSourceProvider = FutureProvider.autoDispose.family<AppSourceInfo?, int>((ref, appId) async {
  final api = ref.watch(apiProvider);
  try {
    return AppSourceInfo.fromJson(await api.get('/apps/$appId/source'));
  } on ApiException catch (e) {
    if (e.status == 404) return null;
    rethrow;
  }
});

/// Builds of an app, refreshed every 3 seconds while one runs.
final buildsProvider = FutureProvider.autoDispose.family<List<BuildInfo>, int>((ref, appId) async {
  final api = ref.watch(apiProvider);
  final list = listOf(await api.get('/apps/$appId/builds'), BuildInfo.fromJson);
  if (list.any((b) => b.running)) {
    final timer = Timer(const Duration(seconds: 3), ref.invalidateSelf);
    ref.onDispose(timer.cancel);
  }
  return list;
});

final registriesProvider = FutureProvider.autoDispose<RegistrySettings>((ref) async {
  final api = ref.watch(apiProvider);
  return RegistrySettings.fromJson(await api.get('/settings/registries'));
});
