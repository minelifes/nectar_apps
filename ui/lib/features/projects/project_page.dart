import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/tokens.dart';
import '../../widgets/activity.dart';
import '../../widgets/app_tile.dart';
import '../../widgets/common.dart';
import '../../widgets/gauge.dart';
import '../access/members_card.dart';
import '../addons/addons_section.dart';
import 'blueprint_dialogs.dart';
import 'env_groups_card.dart';
import 'environments.dart';

class ProjectPage extends ConsumerWidget {
  const ProjectPage({super.key, required this.id});

  final int id;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final project = ref.watch(projectProvider(id));
    final apps = ref.watch(projectAppsProvider(id));
    final activity = ref.watch(projectActivityProvider(id));
    Future<void> refresh() async {
      ref.invalidate(projectActivityProvider(id));
      ref.invalidate(projectProvider(id));
      ref.invalidate(projectAppsProvider(id));
      ref.invalidate(projectAddonsProvider(id));
    }

    final canCreate = ref.watch(sessionProvider.select((s) => s.can('app.create', projectId: id)));
    final back = IconButton(
      tooltip: 'All projects',
      icon: const Icon(Icons.arrow_back_rounded),
      onPressed: () => context.go('/projects'),
    );

    return project.when(
      skipLoadingOnRefresh: true,
      loading: () => const LoadingBlock(height: 320),
      error: (e, _) => PageScaffold(title: 'Project', leading: back, children: [ErrorBlock(error: e, onRetry: refresh)]),
      data: (p) => PageScaffold(
        title: p.name,
        subtitle: p.adopted ? '${p.namespace} · adopted namespace' : p.namespace,
        leading: back,
        onRefresh: refresh,
        actions: [
          if (canCreate)
            OutlinedButton.icon(
              onPressed: () => showTemplateGallery(context, id),
              icon: const Icon(Icons.auto_awesome_outlined, size: 18),
              label: const Text('One-click app'),
            ),
          if (canCreate)
            FilledButton.icon(
              onPressed: () => context.go('/apps/new?project=$id'),
              icon: const Icon(Icons.add_rounded, size: 18),
              label: const Text('New app'),
            ),
          PopupMenuButton<String>(
            tooltip: 'Import / export',
            icon: const Icon(Icons.more_vert_rounded),
            onSelected: (v) => switch (v) {
              'compose' => showImportDialog(context, id),
              'yaml' => showImportDialog(context, id, compose: false),
              'export' => showExportDialog(context, ref, id, p.slug),
              'env' => showCloneProjectDialog(context, p),
              _ => null,
            },
            itemBuilder: (_) => [
              if (canCreate) const PopupMenuItem(value: 'compose', child: Text('Import docker-compose.yml')),
              if (canCreate) const PopupMenuItem(value: 'yaml', child: Text('Apply nectar.yaml')),
              const PopupMenuItem(value: 'export', child: Text('Export nectar.yaml')),
              const PopupMenuItem(value: 'env', child: Text('New environment (copy)…')),
            ],
          ),
        ],
        children: [
          LayoutBuilder(
            builder: (context, box) {
              final quota = Column(
                children: [
                  _QuotaCard(project: p, apps: apps.hasValue ? apps.value! : const []),
                  const SizedBox(height: NectarTokens.gap),
                  MembersCard(projectId: id),
                  const SizedBox(height: NectarTokens.gap),
                  EnvGroupsCard(projectId: id),
                  if (activity.hasValue) ...[
                    const SizedBox(height: NectarTokens.gap),
                    ActivityCard(entries: activity.value!, title: 'Activity'),
                  ],
                ],
              );
              final appsSection = Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _AppsSection(projectId: id, canCreate: canCreate, apps: apps, onRetry: refresh),
                  const SizedBox(height: NectarTokens.gapLarge),
                  AddonsSection(projectId: id, canCreate: canCreate),
                ],
              );
              if (box.maxWidth < 1000) {
                return Column(children: [quota, const SizedBox(height: NectarTokens.gapLarge), appsSection]);
              }
              return Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(child: appsSection),
                  const SizedBox(width: NectarTokens.gapLarge),
                  SizedBox(width: 320, child: quota),
                ],
              );
            },
          ),
        ],
      ),
    );
  }
}

class _AppsSection extends StatelessWidget {
  const _AppsSection({required this.projectId, required this.canCreate, required this.apps, required this.onRetry});

  final int projectId;
  final bool canCreate;
  final AsyncValue<List<App>> apps;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionLabel('Apps'),
        AsyncBlock(
          value: apps,
          onRetry: onRetry,
          data: (list) => list.isEmpty
              ? NCard(
                  child: EmptyState(
                    icon: Icons.add_box_rounded,
                    title: 'No apps in this project yet',
                    message: 'Deploy a JAR, a binary, a static site, Node, Python, PHP or any Docker image.',
                    action: canCreate
                        ? FilledButton.icon(
                            onPressed: () => context.go('/apps/new?project=$projectId'),
                            icon: const Icon(Icons.add_rounded, size: 18),
                            label: const Text('New app'),
                          )
                        : null,
                  ),
                )
              : Wrap(
                  spacing: 12,
                  runSpacing: 12,
                  children: [for (final a in list) AppTile(app: a, onTap: () => context.go('/apps/${a.id}'))],
                ),
        ),
      ],
    );
  }
}

/// Requested resources of all apps against the project quota (null quota = unlimited).
class _QuotaCard extends StatelessWidget {
  const _QuotaCard({required this.project, required this.apps});

  final Project project;
  final List<App> apps;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final q = project.quota;
    final cpu = apps.fold<int>(0, (s, a) => s + a.requestedCpu);
    final mem = apps.fold<int>(0, (s, a) => s + a.requestedMemoryMb);
    double ratio(int used, int limit) => limit <= 0 ? 0 : used / limit;
    String of(String used, int limit, String Function(int) fmt) => limit <= 0 ? '$used · no limit' : '$used / ${fmt(limit)}';

    return NCard(
      title: 'Resources',
      trailing: Text(q == null ? 'Unlimited' : 'Quota', style: TextStyle(color: c.muted, fontSize: 12.5)),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          UsageBar(
            label: 'CPU requested',
            value: ratio(cpu, q?.cpuMillicores ?? 0),
            unlimited: (q?.cpuMillicores ?? 0) <= 0,
            trailing: of(formatCpu(cpu), q?.cpuMillicores ?? 0, formatCpu),
          ),
          const SizedBox(height: 16),
          UsageBar(
            label: 'Memory requested',
            value: ratio(mem, q?.memoryMb ?? 0),
            unlimited: (q?.memoryMb ?? 0) <= 0,
            trailing: of(formatMb(mem), q?.memoryMb ?? 0, formatMb),
          ),
          const SizedBox(height: 16),
          UsageBar(
            label: 'Apps',
            value: ratio(apps.length, q?.apps ?? 0),
            unlimited: (q?.apps ?? 0) <= 0,
            trailing: of('${apps.length}', q?.apps ?? 0, (n) => '$n'),
          ),
          const SizedBox(height: 14),
          Text(
            'Requests of all replicas. Live usage arrives with the metrics view.',
            style: TextStyle(color: c.muted, fontSize: 12),
          ),
        ],
      ),
    );
  }
}
