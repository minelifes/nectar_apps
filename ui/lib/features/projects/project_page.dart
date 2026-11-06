import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/activity.dart';
import '../../widgets/app_tile.dart';
import '../../widgets/common.dart';
import '../../widgets/gauge.dart';
import '../access/members_card.dart';
import '../addons/addons_section.dart';
import 'external_workloads.dart';
import 'blueprint_dialogs.dart';
import 'env_groups_card.dart';
import 'environments.dart';

class ProjectPage extends ConsumerWidget {
  const ProjectPage({super.key, required this.id});

  final int id;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final project = ref.watch(projectProvider(id));
    final apps = ref.watch(projectAppsProvider(id));
    final activity = ref.watch(projectActivityProvider(id));
    Future<void> refresh() async {
      ref.invalidate(projectActivityProvider(id));
      ref.invalidate(projectProvider(id));
      ref.invalidate(projectAppsProvider(id));
      ref.invalidate(projectAddonsProvider(id));
    }

    final canCreate = ref.watch(
      sessionProvider.select((s) => s.can('app.create', projectId: id)),
    );
    final canDelete = ref.watch(
      sessionProvider.select((s) => s.can('project.delete', projectId: id)),
    );
    final back = IconButton(
      tooltip: l.projectAllProjects,
      icon: const Icon(Icons.arrow_back_rounded),
      onPressed: () => context.go('/projects'),
    );

    return project.when(
      skipLoadingOnRefresh: true,
      loading: () => const LoadingBlock(height: 320),
      error: (e, _) => PageScaffold(
        title: l.projectProject,
        leading: back,
        children: [ErrorBlock(error: e, onRetry: refresh)],
      ),
      data: (p) => PageScaffold(
        title: p.name,
        subtitle: p.adopted
            ? '${p.namespace} · adopted namespace'
            : p.namespace,
        leading: back,
        onRefresh: refresh,
        actions: [
          if (canCreate)
            OutlinedButton.icon(
              onPressed: () => showTemplateGallery(context, id),
              icon: const Icon(Icons.auto_awesome_outlined, size: 18),
              label: Text(l.oneClickApp),
            ),
          if (canCreate)
            FilledButton.icon(
              onPressed: () => context.go('/apps/new?project=$id'),
              icon: const Icon(Icons.add_rounded, size: 18),
              label: Text(l.newApp),
            ),
          PopupMenuButton<String>(
            tooltip: l.projectImportExport,
            icon: const Icon(Icons.more_vert_rounded),
            onSelected: (v) => switch (v) {
              'compose' => showImportDialog(context, id),
              'yaml' => showImportDialog(context, id, compose: false),
              'export' => showExportDialog(context, ref, id, p.slug),
              'env' => showCloneProjectDialog(context, p),
              'delete' => _deleteProject(context, ref, p),
              _ => null,
            },
            itemBuilder: (_) => [
              if (canCreate)
                PopupMenuItem(
                  value: 'compose',
                  child: Text(l.projectImportDockerComposeYml),
                ),
              if (canCreate)
                PopupMenuItem(
                  value: 'yaml',
                  child: Text(l.projectApplyNectarYaml),
                ),
              PopupMenuItem(
                value: 'export',
                child: Text(l.projectExportNectarYaml),
              ),
              PopupMenuItem(
                value: 'env',
                child: Text(l.projectNewEnvironmentCopy),
              ),
              if (canDelete) const PopupMenuDivider(),
              if (canDelete)
                PopupMenuItem(
                  value: 'delete',
                  child: Text(
                    l.projectDeleteProject,
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.error,
                    ),
                  ),
                ),
            ],
          ),
        ],
        children: [
          LayoutBuilder(
            builder: (context, box) {
              final quota = Column(
                children: [
                  _QuotaCard(
                    project: p,
                    apps: apps.hasValue ? apps.value! : const [],
                  ),
                  const SizedBox(height: NectarTokens.gap),
                  MembersCard(projectId: id),
                  const SizedBox(height: NectarTokens.gap),
                  EnvGroupsCard(projectId: id),
                  if (activity.hasValue) ...[
                    const SizedBox(height: NectarTokens.gap),
                    ActivityCard(
                      entries: activity.value!,
                      title: l.projectActivity,
                    ),
                  ],
                ],
              );
              final appsSection = Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  // apps marked as databases are listed with the databases
                  _AppsSection(
                    projectId: id,
                    canCreate: canCreate,
                    apps: apps.whenData(
                      (l) => l.where((a) => a.database == null).toList(),
                    ),
                    onRetry: refresh,
                  ),
                  const SizedBox(height: NectarTokens.gapLarge),
                  AddonsSection(
                    projectId: id,
                    canCreate: canCreate,
                    databaseApps: [
                      for (final a in apps.value ?? const <App>[])
                        if (a.database != null) a,
                    ],
                  ),
                  if (p.adopted) ProjectExternalWorkloads(projectId: id),
                ],
              );
              if (box.maxWidth < 1000) {
                return Column(
                  children: [
                    appsSection,
                    const SizedBox(height: NectarTokens.gapLarge),
                    quota,
                  ],
                );
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
  const _AppsSection({
    required this.projectId,
    required this.canCreate,
    required this.apps,
    required this.onRetry,
  });

  final int projectId;
  final bool canCreate;
  final AsyncValue<List<App>> apps;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
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
                    title: s.projectNoAppsInThisProject,
                    message: s.projectDeployAJarABinary,
                    action: canCreate
                        ? FilledButton.icon(
                            onPressed: () =>
                                context.go('/apps/new?project=$projectId'),
                            icon: const Icon(Icons.add_rounded, size: 18),
                            label: Text(s.newApp),
                          )
                        : null,
                  ),
                )
              : Wrap(
                  spacing: 12,
                  runSpacing: 12,
                  children: [
                    for (final a in list)
                      AppTile(app: a, onTap: () => context.go('/apps/${a.id}')),
                  ],
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
    final l = S.of(context);
    final c = NectarColors.of(context);
    final q = project.quota;
    final cpu = apps.fold<int>(0, (s, a) => s + a.requestedCpu);
    final mem = apps.fold<int>(0, (s, a) => s + a.requestedMemoryMb);
    double ratio(int used, int limit) => limit <= 0 ? 0 : used / limit;
    String of(String used, int limit, String Function(int) fmt) =>
        limit <= 0 ? '$used · no limit' : '$used / ${fmt(limit)}';

    return NCard(
      title: l.projectResources,
      trailing: Text(
        q == null ? 'Unlimited' : 'Quota',
        style: TextStyle(color: c.muted, fontSize: 12.5),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          UsageBar(
            label: l.projectCpuRequested,
            value: ratio(cpu, q?.cpuMillicores ?? 0),
            unlimited: (q?.cpuMillicores ?? 0) <= 0,
            trailing: of(formatCpu(cpu), q?.cpuMillicores ?? 0, formatCpu),
          ),
          const SizedBox(height: 16),
          UsageBar(
            label: l.projectMemoryRequested,
            value: ratio(mem, q?.memoryMb ?? 0),
            unlimited: (q?.memoryMb ?? 0) <= 0,
            trailing: of(formatMb(mem), q?.memoryMb ?? 0, formatMb),
          ),
          const SizedBox(height: 16),
          UsageBar(
            label: l.projectApps,
            value: ratio(apps.length, q?.apps ?? 0),
            unlimited: (q?.apps ?? 0) <= 0,
            trailing: of('${apps.length}', q?.apps ?? 0, (n) => '$n'),
          ),
          const SizedBox(height: 14),
          Text(
            l.projectRequestsOfAllReplicasLive,
            style: TextStyle(color: c.muted, fontSize: 12),
          ),
        ],
      ),
    );
  }
}

/// Deletes a project after confirmation. A project the panel created loses its namespace and everything in
/// it (the slug must be typed); an adopted one only stops being a project: its own workloads keep running.
Future<void> _deleteProject(
  BuildContext context,
  WidgetRef ref,
  Project p,
) async {
  final s = S.of(context);
  if (p.adopted) {
    if (!await confirmAction(
      context,
      s.projectDeleteTitle(p.name),
      s.projectDeleteAdoptedBody(p.namespace),
      s.actionDelete,
      danger: true,
    )) {
      return;
    }
  } else {
    final typed = await promptText(
      context,
      s.projectDeleteTitle(p.name),
      s.projectDeleteTypeSlug(p.slug),
    );
    if (typed == null) return;
    if (typed.trim() != p.slug) {
      if (context.mounted) {
        showSnack(context, s.projectDeleteSlugMismatch, error: true);
      }
      return;
    }
  }
  if (!context.mounted) return;
  try {
    await ref.read(apiProvider).delete('/projects/${p.id}');
    ref.invalidate(projectsProvider);
    if (!context.mounted) return;
    showSnack(context, s.projectDeleted(p.name));
    context.go('/projects');
  } on Object catch (e) {
    if (context.mounted) showSnack(context, describeError(e), error: true);
  }
}
