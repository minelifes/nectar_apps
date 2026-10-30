import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../apps/env_editor.dart';
import '../apps/resources.dart';

/// Env groups of a project: variables shared by several apps (each app picks its groups on its
/// Environment tab; its own variables win). Reference a value elsewhere with ${group.KEY}.
class EnvGroupsCard extends ConsumerWidget {
  const EnvGroupsCard({super.key, required this.projectId});

  final int projectId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = NectarColors.of(context);
    final groups = ref.watch(envGroupsProvider(projectId));
    final canEdit = ref.watch(sessionProvider.select((s) => s.can('env.edit', projectId: projectId)));
    return NCard(
      title: 'Env groups',
      trailing: canEdit
          ? TextButton.icon(
              onPressed: () => showEnvGroupDialog(context, projectId, null),
              icon: const Icon(Icons.add_rounded, size: 18),
              label: const Text('New group'),
            )
          : null,
      child: AsyncBlock(
        value: groups,
        loadingHeight: 60,
        onRetry: () => ref.invalidate(envGroupsProvider(projectId)),
        data: (list) => list.isEmpty
            ? Text('Variables several apps share (a region, an API endpoint, a Sentry DSN). Apps choose their groups on the Environment tab.',
                style: TextStyle(color: c.muted, fontSize: 12.5))
            : Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (final g in list)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      dense: true,
                      leading: Icon(Icons.dataset_outlined, color: c.muted, size: 20),
                      title: Text(g.name, style: monoStyle(context, size: 13.5)),
                      subtitle: Text(
                        '${g.vars.length} variable${g.vars.length == 1 ? '' : 's'} · ${g.apps.isEmpty ? 'no apps yet' : g.apps.join(', ')}',
                        style: TextStyle(color: c.muted, fontSize: 12),
                      ),
                      onTap: canEdit ? () => showEnvGroupDialog(context, projectId, g) : null,
                    ),
                ],
              ),
      ),
    );
  }
}

Future<void> showEnvGroupDialog(BuildContext context, int projectId, EnvGroupInfo? group) =>
    showDialog<void>(context: context, builder: (_) => _EnvGroupDialog(projectId: projectId, group: group));

class _EnvGroupDialog extends ConsumerStatefulWidget {
  const _EnvGroupDialog({required this.projectId, required this.group});

  final int projectId;
  final EnvGroupInfo? group;

  @override
  ConsumerState<_EnvGroupDialog> createState() => _EnvGroupDialogState();
}

class _EnvGroupDialogState extends ConsumerState<_EnvGroupDialog> {
  late final _name = TextEditingController(text: widget.group?.name ?? '');
  late final List<EnvVar> _vars = [for (final v in widget.group?.vars ?? const <EnvVar>[]) EnvVar(key: v.key, value: v.value, secret: v.secret)];
  bool _busy = false;
  int _gen = 0;

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final vars = _vars.where((v) => v.key.isNotEmpty || (v.value ?? '').isNotEmpty).toList();
    for (final v in vars) {
      if (!envKeyRe.hasMatch(v.key)) {
        showSnack(context, 'Invalid variable name "${v.key}": use letters, digits and _', error: true);
        return;
      }
    }
    setState(() => _busy = true);
    try {
      await ref.read(apiProvider).put('/projects/${widget.projectId}/env-groups/${_name.text.trim()}', data: [for (final v in vars) v.toJson()]);
      ref.invalidate(envGroupsProvider(widget.projectId));
      if (mounted) {
        Navigator.pop(context);
        showSnack(context, widget.group?.apps.isNotEmpty ?? false ? 'Saved. Redeploy ${widget.group!.apps.join(', ')} to apply.' : 'Saved');
      }
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _delete() async {
    final g = widget.group!;
    if (!await confirmAction(context, 'Delete the group ${g.name}?', 'Its variables are deleted.', 'Delete', danger: true)) return;
    try {
      await ref.read(apiProvider).delete('/projects/${widget.projectId}/env-groups/${g.name}');
      ref.invalidate(envGroupsProvider(widget.projectId));
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _reveal() async {
    try {
      final all = listOf(await ref.read(apiProvider).get('/projects/${widget.projectId}/env-groups', query: {'reveal': '1'}), EnvGroupInfo.fromJson);
      final g = all.where((x) => x.name == widget.group?.name).firstOrNull;
      if (g == null) return;
      final byKey = {for (final v in g.vars) v.key: v.value};
      setState(() {
        for (final v in _vars) {
          if (v.value == null && byKey[v.key] != null) v.value = byKey[v.key];
        }
        _gen++;
      });
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final g = widget.group;
    return AlertDialog(
      title: Text(g == null ? 'New env group' : 'Env group ${g.name}'),
      content: SizedBox(
        width: 640,
        child: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              if (g == null) ...[
                TextField(controller: _name, decoration: const InputDecoration(labelText: 'Name', hintText: 'shared')),
                const SizedBox(height: 12),
              ],
              EnvEditor(key: ValueKey(_gen), vars: _vars, onChanged: () => setState(() {})),
              if (_vars.any((v) => v.secret && v.value == null))
                Align(alignment: Alignment.centerLeft, child: TextButton(onPressed: _reveal, child: const Text('Show secret values'))),
              const SizedBox(height: 8),
              Text(
                g == null || g.apps.isEmpty
                    ? 'Apps pick groups on their Environment tab. Elsewhere, reference a value as \${${g?.name ?? 'group'}.KEY}.'
                    : 'Used by ${g.apps.join(', ')} — changes apply with their next deploy.',
                style: TextStyle(color: c.muted, fontSize: 12.5),
              ),
            ],
          ),
        ),
      ),
      actions: [
        if (g != null) TextButton(onPressed: _busy ? null : _delete, style: TextButton.styleFrom(foregroundColor: c.failed), child: const Text('Delete')),
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _save, child: Text(_busy ? 'Saving…' : 'Save')),
      ],
    );
  }
}

/// The env groups an app gets (chips on its Environment tab).
class AppEnvGroups extends ConsumerStatefulWidget {
  const AppEnvGroups({super.key, required this.app, required this.canEdit});

  final App app;
  final bool canEdit;

  @override
  ConsumerState<AppEnvGroups> createState() => _AppEnvGroupsState();
}

class _AppEnvGroupsState extends ConsumerState<AppEnvGroups> {
  late List<String> _chosen = widget.app.envGroups;
  bool _busy = false;

  Future<void> _toggle(String name, bool on) async {
    final next = on ? [..._chosen, name] : _chosen.where((n) => n != name).toList();
    setState(() => _busy = true);
    try {
      await ref.read(apiProvider).put('/apps/${widget.app.id}/env-groups', data: next);
      setState(() => _chosen = next);
      ref.invalidate(appProvider(widget.app.id));
      ref.invalidate(appLiveProvider(widget.app.id));
      ref.invalidate(envGroupsProvider(widget.app.projectId));
      if (mounted) showSnack(context, 'Saved. Redeploy the app to apply.');
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final groups = ref.watch(envGroupsProvider(widget.app.projectId));
    return NCard(
      title: 'Shared env groups',
      child: AsyncBlock(
        value: groups,
        loadingHeight: 40,
        data: (list) => list.isEmpty
            ? Text('This project has no env groups yet (project page → Env groups).', style: TextStyle(color: c.muted, fontSize: 12.5))
            : Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Wrap(
                    spacing: 8,
                    runSpacing: 6,
                    children: [
                      for (final g in list)
                        FilterChip(
                          label: Text('${g.name} (${g.vars.length})'),
                          selected: _chosen.contains(g.name),
                          onSelected: widget.canEdit && !_busy ? (v) => _toggle(g.name, v) : null,
                        ),
                    ],
                  ),
                  const SizedBox(height: 6),
                  Text('The app gets every variable of the chosen groups; its own variables win.', style: TextStyle(color: c.muted, fontSize: 12)),
                ],
              ),
      ),
    );
  }
}
