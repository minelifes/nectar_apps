import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/hex.dart';
import 'new_project_dialog.dart';

class ProjectsPage extends ConsumerStatefulWidget {
  const ProjectsPage({super.key});

  @override
  ConsumerState<ProjectsPage> createState() => _ProjectsPageState();
}

class _ProjectsPageState extends ConsumerState<ProjectsPage> {
  String _query = '';

  @override
  Widget build(BuildContext context) {
    final projects = ref.watch(projectsProvider);
    final canCreate = ref.watch(sessionProvider.select((s) => s.can('project.create')));
    return PageScaffold(
      title: 'Projects',
      subtitle: 'Each project has its own Kubernetes namespace and optional quota.',
      onRefresh: () async => ref.invalidate(projectsProvider),
      actions: [
        if (canCreate)
          FilledButton.icon(
            onPressed: () => showNewProjectDialog(context, ref),
            icon: const Icon(Icons.add_rounded, size: 18),
            label: const Text('New project'),
          ),
      ],
      children: [
        AsyncBlock(
          value: projects,
          onRetry: () => ref.invalidate(projectsProvider),
          data: (list) {
            if (list.isEmpty) {
              return NCard(
                child: EmptyState(
                  icon: Icons.folder_open_rounded,
                  title: 'No projects yet',
                  message: canCreate ? 'Create a project to start deploying apps.' : 'Ask an Owner to give you access to a project.',
                ),
              );
            }
            final q = _query.toLowerCase();
            final shown = q.isEmpty
                ? list
                : list.where((p) => p.name.toLowerCase().contains(q) || p.slug.contains(q)).toList();
            return Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (list.length > 6)
                  Padding(
                    padding: const EdgeInsets.only(bottom: NectarTokens.gap),
                    child: SizedBox(
                      width: 360,
                      child: TextField(
                        decoration: const InputDecoration(prefixIcon: Icon(Icons.search_rounded, size: 20), hintText: 'Search projects'),
                        onChanged: (v) => setState(() => _query = v.trim()),
                      ),
                    ),
                  ),
                Wrap(
                  spacing: NectarTokens.gap,
                  runSpacing: NectarTokens.gap,
                  children: [for (final p in shown) _ProjectCard(project: p)],
                ),
              ],
            );
          },
        ),
      ],
    );
  }
}

class _ProjectCard extends StatelessWidget {
  const _ProjectCard({required this.project});

  final Project project;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final theme = Theme.of(context);
    final q = project.quota;
    return SizedBox(
      width: 340,
      child: NCard(
        onTap: () => context.go('/projects/${project.id}'),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                HexBadge(
                  size: 40,
                  color: NectarTokens.amber.withValues(alpha: 0.16),
                  child: Text(
                    project.name.isEmpty ? '?' : project.name.substring(0, 1).toUpperCase(),
                    style: const TextStyle(color: NectarTokens.amber, fontWeight: FontWeight.w700, fontSize: 15),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(project.name, maxLines: 1, overflow: TextOverflow.ellipsis, style: theme.textTheme.titleMedium),
                      Text(project.namespace, style: monoStyle(context, size: 12, color: c.muted)),
                    ],
                  ),
                ),
                if (project.adopted)
                  const Tooltip(
                    message: 'Adopted from an existing namespace: other workloads there are left alone.',
                    child: Chip(label: Text('Adopted'), visualDensity: VisualDensity.compact),
                  ),
              ],
            ),
            const SizedBox(height: 16),
            Row(
              children: [
                Icon(Icons.apps_rounded, size: 16, color: c.muted),
                const SizedBox(width: 6),
                Text('${project.apps} ${project.apps == 1 ? 'app' : 'apps'}', style: const TextStyle(fontSize: 13.5)),
                const SizedBox(width: 16),
                Icon(Icons.speed_rounded, size: 16, color: c.muted),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    q == null ? 'Unlimited' : _quotaSummary(q),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(fontSize: 13.5),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  static String _quotaSummary(Quota q) => [
        if (q.cpuMillicores > 0) formatCpu(q.cpuMillicores),
        if (q.memoryMb > 0) formatMb(q.memoryMb),
        if (q.apps > 0) '${q.apps} apps max',
      ].join(' · ');
}
