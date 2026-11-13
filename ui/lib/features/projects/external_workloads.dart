import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import 'import_workload_dialog.dart';

/// What runs in a namespace the panel did not create (an existing k3s cluster adopted by Nectar): read-only.
class ExternalWorkload {
  const ExternalWorkload({
    required this.kind,
    required this.name,
    required this.replicas,
    required this.ready,
    required this.images,
  });

  final String kind; // Deployment | StatefulSet | DaemonSet
  final String name;
  final int replicas;
  final int ready;
  final List<String> images;

  factory ExternalWorkload.fromJson(Map m) => ExternalWorkload(
    kind: '${m['kind'] ?? ''}',
    name: '${m['name'] ?? ''}',
    replicas: (m['replicas'] as num?)?.toInt() ?? 0,
    ready: (m['ready'] as num?)?.toInt() ?? 0,
    images: [for (final i in (m['images'] as List? ?? const [])) '$i'],
  );
}

class ExternalNamespace {
  const ExternalNamespace({
    required this.name,
    required this.workloads,
    required this.services,
    required this.hosts,
    this.projectId,
  });

  final String name;
  final List<ExternalWorkload> workloads;
  final List<(String name, String type, List<int> ports)> services;
  final List<String> hosts;
  final int? projectId; // set when it was adopted as a project

  factory ExternalNamespace.fromJson(Map m) => ExternalNamespace(
    name: '${m['name'] ?? ''}',
    workloads: [
      for (final w in (m['workloads'] as List? ?? const []))
        ExternalWorkload.fromJson(w as Map),
    ],
    services: [
      for (final s in (m['services'] as List? ?? const []))
        (
          '${(s as Map)['name']}',
          '${s['type']}',
          [
            for (final p in (s['ports'] as List? ?? const []))
              (p as num).toInt(),
          ],
        ),
    ],
    hosts: [for (final h in (m['hosts'] as List? ?? const [])) '$h'],
    projectId: (m['project_id'] as num?)?.toInt(),
  );
}

/// Namespaces of the cluster that are not Nectar projects (server.view).
final externalNamespacesProvider =
    FutureProvider.autoDispose<List<ExternalNamespace>>((ref) async {
      final res = await ref.watch(apiProvider).get('/external/namespaces');
      return [
        for (final n in (res as List)) ExternalNamespace.fromJson(n as Map),
      ];
    });

/// What already runs in an adopted project's namespace.
final projectExternalProvider = FutureProvider.autoDispose
    .family<ExternalNamespace, int>((ref, projectId) async {
      final res = await ref
          .watch(apiProvider)
          .get('/projects/$projectId/external-workloads');
      return ExternalNamespace.fromJson(res as Map);
    });

/// Projects page: workloads that ran on the cluster before Nectar (or next to it), with "Adopt as project".
class ExternalNamespacesCard extends ConsumerWidget {
  const ExternalNamespacesCard({super.key});

  Future<void> _adopt(
    BuildContext context,
    WidgetRef ref,
    ExternalNamespace ns,
  ) async {
    final s = S.of(context);
    final ok = await confirmAction(
      context,
      s.externalAdoptTitle(ns.name),
      s.externalAdoptBody,
      s.externalAdopt,
    );
    if (!ok || !context.mounted) return;
    try {
      final res = await ref
          .read(apiProvider)
          .post(
            '/external/namespaces/${ns.name}/adopt',
            data: {'name': ns.name},
          );
      ref.invalidate(projectsProvider);
      ref.invalidate(externalNamespacesProvider);
      final id = ((res as Map)['id'] as num).toInt();
      if (context.mounted) context.go('/projects/$id');
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final session = ref.watch(sessionProvider);
    if (!session.can('server.view')) return const SizedBox.shrink();
    final list = ref.watch(externalNamespacesProvider);
    final canAdopt = session.can('project.create');
    final c = NectarColors.of(context);
    final shown =
        list.value?.where((n) => n.projectId == null).toList() ?? const [];
    if (shown.isEmpty) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(top: NectarTokens.gapLarge),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SectionLabel(s.externalOnThisCluster),
          Padding(
            padding: const EdgeInsets.only(bottom: 10),
            child: Text(
              s.externalOnThisClusterHelp,
              style: TextStyle(color: c.muted, fontSize: 12.5),
            ),
          ),
          for (final ns in shown)
            Padding(
              padding: const EdgeInsets.only(bottom: NectarTokens.gap),
              child: NCard(
                title: ns.name,
                trailing: canAdopt
                    ? TextButton.icon(
                        onPressed: () => _adopt(context, ref, ns),
                        icon: const Icon(Icons.input_rounded, size: 18),
                        label: Text(s.externalAdopt),
                      )
                    : null,
                child: ExternalNamespaceBody(ns: ns),
              ),
            ),
        ],
      ),
    );
  }
}

/// Project page of an adopted namespace: what runs there without being a Nectar app.
class ProjectExternalWorkloads extends ConsumerWidget {
  const ProjectExternalWorkloads({super.key, required this.projectId});

  final int projectId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final ns = ref.watch(projectExternalProvider(projectId)).value;
    if (ns == null || (ns.workloads.isEmpty && ns.hosts.isEmpty)) {
      return const SizedBox.shrink();
    }
    return Padding(
      padding: const EdgeInsets.only(top: NectarTokens.gapLarge),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SectionLabel(s.externalNotManaged),
          Padding(
            padding: const EdgeInsets.only(bottom: 10),
            child: Text(
              s.externalNotManagedHelp,
              style: TextStyle(color: c.muted, fontSize: 12.5),
            ),
          ),
          NCard(
            child: ExternalNamespaceBody(
              ns: ns,
              onImport:
                  ref
                      .watch(sessionProvider)
                      .can('app.create', projectId: projectId)
                  ? (w) => showImportWorkloadDialog(context, projectId, w)
                  : null,
            ),
          ),
        ],
      ),
    );
  }
}

/// The workloads, Services and hosts of an external namespace.
class ExternalNamespaceBody extends StatelessWidget {
  const ExternalNamespaceBody({super.key, required this.ns, this.onImport});

  final ExternalNamespace ns;

  /// Shows "Import as app" on Deployments and StatefulSets (adopted projects).
  final void Function(ExternalWorkload w)? onImport;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final small = TextStyle(color: c.muted, fontSize: 12.5);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (ns.workloads.isEmpty) Text(s.externalNoWorkloads, style: small),
        for (final w in ns.workloads)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 6),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(
                  w.ready >= w.replicas && w.replicas > 0
                      ? Icons.check_circle_rounded
                      : Icons.error_outline_rounded,
                  size: 18,
                  color: w.ready >= w.replicas && w.replicas > 0
                      ? c.running
                      : (w.replicas == 0 ? c.muted : c.warning),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '${w.name}  ·  ${w.kind}',
                        style: const TextStyle(fontWeight: FontWeight.w600),
                      ),
                      Text(
                        w.images.join(', '),
                        style: monoStyle(context, size: 12, color: c.muted),
                      ),
                    ],
                  ),
                ),
                Text(s.externalReady(w.ready, w.replicas), style: small),
                if (onImport != null &&
                    (w.kind == 'Deployment' || w.kind == 'StatefulSet')) ...[
                  const SizedBox(width: 8),
                  TextButton(
                    onPressed: () => onImport!(w),
                    child: Text(s.importAction),
                  ),
                ],
              ],
            ),
          ),
        if (ns.services.isNotEmpty) ...[
          const Divider(height: 18),
          Text(
            [
              for (final (name, type, ports) in ns.services)
                '$name ($type ${ports.join(', ')})',
            ].join('   '),
            style: monoStyle(context, size: 12, color: c.muted),
          ),
        ],
        if (ns.hosts.isNotEmpty) ...[
          const SizedBox(height: 6),
          Text(
            ns.hosts.join('   '),
            style: monoStyle(context, size: 12, color: c.muted),
          ),
        ],
      ],
    );
  }
}
