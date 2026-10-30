import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Projects linked as environments of [p]: the one it was copied from, its copies, and their siblings.
List<Project> relatedProjects(Project p, List<Project> all) => [
      for (final o in all)
        if (o.id != p.id &&
            (o.id == p.originId || o.originId == p.id || (p.originId != null && o.originId == p.originId)))
          o,
    ];

String envLabel(Project p) => p.environment.isEmpty ? p.slug : '${p.slug} (${p.environment})';

/// Copy an app (settings, variables, config files, env groups) into this or another project.
Future<void> showCloneAppDialog(BuildContext context, App app) =>
    showDialog<void>(context: context, builder: (_) => _CloneAppDialog(app: app));

class _CloneAppDialog extends ConsumerStatefulWidget {
  const _CloneAppDialog({required this.app});

  final App app;

  @override
  ConsumerState<_CloneAppDialog> createState() => _CloneAppDialogState();
}

class _CloneAppDialogState extends ConsumerState<_CloneAppDialog> {
  late final _slug = TextEditingController(text: '${widget.app.slug}-copy');
  late final _name = TextEditingController(text: '${widget.app.name} (copy)');
  late int _project = widget.app.projectId;
  bool _busy = false;

  @override
  void dispose() {
    _slug.dispose();
    _name.dispose();
    super.dispose();
  }

  Future<void> _copy() async {
    setState(() => _busy = true);
    try {
      final res = await ref.read(apiProvider).post('/apps/${widget.app.id}/clone',
          data: {'slug': _slug.text.trim(), 'name': _name.text.trim(), 'project_id': _project}) as Map;
      ref.invalidate(projectAppsProvider(_project));
      final copied = res['app'] as Map;
      final notes = [for (final n in (res['notes'] as List? ?? const [])) '$n'];
      if (!mounted) return;
      Navigator.pop(context);
      showSnack(context, notes.isEmpty ? 'Copied — deploy it when ready' : 'Copied. ${notes.join(' · ')}');
      context.go('/apps/${copied['id']}');
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final projects = ref.watch(projectsProvider).value ?? const <Project>[];
    final session = ref.watch(sessionProvider);
    return AlertDialog(
      title: Text('Copy ${widget.app.name}'),
      content: SizedBox(
        width: 460,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            DropdownButtonFormField<int>(
              initialValue: _project,
              decoration: const InputDecoration(labelText: 'Into project'),
              items: [
                for (final p in projects)
                  if (session.can('app.create', projectId: p.id)) DropdownMenuItem(value: p.id, child: Text(envLabel(p))),
              ],
              onChanged: (v) => setState(() => _project = v ?? _project),
            ),
            const SizedBox(height: 10),
            TextField(controller: _slug, decoration: const InputDecoration(labelText: 'Slug')),
            const SizedBox(height: 10),
            TextField(controller: _name, decoration: const InputDecoration(labelText: 'Name')),
            const SizedBox(height: 10),
            Text('Copies the settings, variables (secrets too), config files and env groups. Volumes start empty; nothing is deployed.',
                style: TextStyle(color: c.muted, fontSize: 12.5)),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _copy, child: Text(_busy ? 'Copying…' : 'Copy')),
      ],
    );
  }
}

/// Copy a project as another environment (e.g. staging): env groups, databases (new and empty) and apps.
Future<void> showCloneProjectDialog(BuildContext context, Project project) =>
    showDialog<void>(context: context, builder: (_) => _CloneProjectDialog(project: project));

class _CloneProjectDialog extends ConsumerStatefulWidget {
  const _CloneProjectDialog({required this.project});

  final Project project;

  @override
  ConsumerState<_CloneProjectDialog> createState() => _CloneProjectDialogState();
}

class _CloneProjectDialogState extends ConsumerState<_CloneProjectDialog> {
  late final _env = TextEditingController(text: 'staging');
  late final _slug = TextEditingController(text: '${widget.project.slug}-staging');
  late final _sourceEnv = TextEditingController(text: widget.project.environment.isEmpty ? 'production' : widget.project.environment);
  bool _busy = false;

  @override
  void dispose() {
    for (final c in [_env, _slug, _sourceEnv]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _copy() async {
    setState(() => _busy = true);
    try {
      final res = await ref.read(apiProvider).post('/projects/${widget.project.id}/clone', data: {
        'slug': _slug.text.trim(),
        'environment': _env.text.trim(),
        'source_environment': _sourceEnv.text.trim(),
      }) as Map;
      ref.invalidate(projectsProvider);
      ref.invalidate(projectProvider(widget.project.id));
      final p = res['project'] as Map;
      final notes = [for (final n in (res['notes'] as List? ?? const [])) '$n'];
      if (!mounted) return;
      Navigator.pop(context);
      showSnack(context, notes.isEmpty ? 'Created ${p['slug']} — deploy its apps when ready' : 'Created ${p['slug']}. ${notes.join(' · ')}');
      context.go('/projects/${p['id']}');
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return AlertDialog(
      title: const Text('New environment'),
      content: SizedBox(
        width: 460,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextField(
              controller: _env,
              decoration: const InputDecoration(labelText: 'Environment of the copy', hintText: 'staging'),
              onChanged: (v) => _slug.text = '${widget.project.slug}-${v.trim()}',
            ),
            const SizedBox(height: 10),
            TextField(controller: _slug, decoration: const InputDecoration(labelText: 'Project slug of the copy')),
            const SizedBox(height: 10),
            TextField(controller: _sourceEnv, decoration: InputDecoration(labelText: 'Environment of ${widget.project.slug}')),
            const SizedBox(height: 10),
            Text(
              'Copies the env groups, the databases (new and empty, with new passwords) and the apps with their settings, variables '
              'and config files. Nothing is deployed. Later, "Promote" deploys the live build of an app to the same app in the other environment.',
              style: TextStyle(color: c.muted, fontSize: 12.5),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _copy, child: Text(_busy ? 'Copying…' : 'Create')),
      ],
    );
  }
}

/// Deploys the app's live build to the app of the same name in [target].
Future<void> promoteApp(BuildContext context, WidgetRef ref, App app, Project target) async {
  if (!await confirmAction(context, 'Promote ${app.name} to ${envLabel(target)}?',
      'The live build of ${app.slug} (its image or uploaded file) is deployed to ${target.slug}/${app.slug}. Its settings and variables there stay as they are.', 'Promote')) {
    return;
  }
  try {
    final res = await ref.read(apiProvider).post('/apps/${app.id}/promote', data: {'project_id': target.id}) as Map;
    if (!context.mounted) return;
    showSnack(context, 'Deploying ${res['build']} to ${target.slug}');
    context.go('/apps/${res['app_id']}');
  } on Object catch (e) {
    if (context.mounted) showSnack(context, describeError(e), error: true);
  }
}
