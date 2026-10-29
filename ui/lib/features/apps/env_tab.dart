import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import 'env_editor.dart';
import 'resources.dart';

/// Environment variables and config files of an app. Changes apply with the next deploy.
class EnvTab extends ConsumerWidget {
  const EnvTab({super.key, required this.app});

  final App app;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    final canEdit = session.can('env.edit', projectId: app.projectId);
    final canReveal = session.can('env.secrets.view', projectId: app.projectId);
    final env = ref.watch(appEnvProvider(app.id));
    final files = ref.watch(appFilesProvider(app.id));
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        NCard(
          title: 'Environment variables',
          child: AsyncBlock(
            value: env,
            onRetry: () => ref.invalidate(appEnvProvider(app.id)),
            data: (vars) => _EnvCard(key: ValueKey(vars), app: app, initial: vars, canEdit: canEdit, canReveal: canReveal),
          ),
        ),
        const SizedBox(height: NectarTokens.gap),
        _LinkedDatabases(app: app),
        const SizedBox(height: NectarTokens.gap),
        NCard(
          title: 'Config files',
          child: AsyncBlock(
            value: files,
            onRetry: () => ref.invalidate(appFilesProvider(app.id)),
            data: (list) => _FilesCard(key: ValueKey(list), app: app, initial: list, canEdit: canEdit, canReveal: canReveal),
          ),
        ),
      ],
    );
  }
}

void _savedHint(BuildContext context, WidgetRef ref, App app) {
  ref.invalidate(appLiveProvider(app.id)); // shows the "not live yet" banner
  showSnack(context, 'Saved. Redeploy the app to apply.');
}

class _EnvCard extends ConsumerStatefulWidget {
  const _EnvCard({super.key, required this.app, required this.initial, required this.canEdit, required this.canReveal});

  final App app;
  final List<EnvVar> initial;
  final bool canEdit;
  final bool canReveal;

  @override
  ConsumerState<_EnvCard> createState() => _EnvCardState();
}

class _EnvCardState extends ConsumerState<_EnvCard> {
  late List<EnvVar> _vars = [for (final v in widget.initial) EnvVar(key: v.key, value: v.value, secret: v.secret)];
  bool _dirty = false;
  bool _saving = false;
  int _gen = 0; // rebuilds the editor rows after a reveal

  Future<void> _reveal() async {
    try {
      final all = listOf(await ref.read(apiProvider).get('/apps/${widget.app.id}/env', query: {'reveal': '1'}), EnvVar.fromJson);
      final byKey = {for (final v in all) v.key: v.value};
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

  Future<void> _save() async {
    final vars = _vars.where((v) => v.key.isNotEmpty || (v.value ?? '').isNotEmpty).toList();
    for (final v in vars) {
      if (!envKeyRe.hasMatch(v.key)) {
        showSnack(context, 'Invalid variable name "${v.key}": use letters, digits and _', error: true);
        return;
      }
    }
    setState(() => _saving = true);
    try {
      await ref.read(apiProvider).put('/apps/${widget.app.id}/env', data: [for (final v in vars) v.toJson()]);
      if (!mounted) return;
      _savedHint(context, ref, widget.app);
      ref.invalidate(appEnvProvider(widget.app.id));
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    if (!widget.canEdit) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_vars.isEmpty) Text('No variables.', style: TextStyle(color: c.muted)),
          for (final v in _vars)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 3),
              child: Text('${v.key}=${v.secret ? '••••••' : v.value ?? ''}', style: monoStyle(context, size: 13)),
            ),
        ],
      );
    }
    final hidden = _vars.any((v) => v.secret && v.value == null);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        EnvEditor(key: ValueKey(_gen), vars: _vars, onChanged: () => setState(() => _dirty = true)),
        const SizedBox(height: 8),
        Row(
          children: [
            if (hidden && widget.canReveal)
              TextButton.icon(onPressed: _reveal, icon: const Icon(Icons.visibility_outlined, size: 18), label: const Text('Reveal secrets')),
            const Spacer(),
            if (_dirty)
              TextButton(
                onPressed: () => setState(() {
                  _vars = [for (final v in widget.initial) EnvVar(key: v.key, value: v.value, secret: v.secret)];
                  _dirty = false;
                  _gen++;
                }),
                child: const Text('Discard'),
              ),
            const SizedBox(width: 8),
            FilledButton(onPressed: !_dirty || _saving ? null : _save, child: Text(_saving ? 'Saving…' : 'Save variables')),
          ],
        ),
      ],
    );
  }
}

class _FilesCard extends ConsumerStatefulWidget {
  const _FilesCard({super.key, required this.app, required this.initial, required this.canEdit, required this.canReveal});

  final App app;
  final List<AppFile> initial;
  final bool canEdit;
  final bool canReveal;

  @override
  ConsumerState<_FilesCard> createState() => _FilesCardState();
}

class _FilesCardState extends ConsumerState<_FilesCard> {
  late final List<AppFile> _files = List.of(widget.initial);
  bool _saving = false;

  Future<void> _save(List<AppFile> files) async {
    setState(() => _saving = true);
    try {
      await ref.read(apiProvider).put('/apps/${widget.app.id}/files', data: [for (final f in files) f.toJson()]);
      if (!mounted) return;
      _savedHint(context, ref, widget.app);
      ref.invalidate(appFilesProvider(widget.app.id));
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _edit(AppFile? file) async {
    var f = file;
    if (f != null && f.secret && f.content == null && widget.canReveal) {
      try {
        final all = listOf(await ref.read(apiProvider).get('/apps/${widget.app.id}/files', query: {'reveal': '1'}), AppFile.fromJson);
        f = all.firstWhere((x) => x.path == file!.path, orElse: () => file!);
      } on Object catch (e) {
        if (mounted) showSnack(context, describeError(e), error: true);
        return;
      }
    }
    if (!mounted) return;
    final result = await showDialog<AppFile>(context: context, builder: (_) => _FileDialog(file: f, canEdit: widget.canEdit));
    if (result == null) return;
    final list = List.of(_files);
    final i = file == null ? -1 : list.indexOf(file);
    if (i >= 0) {
      list[i] = result;
    } else {
      list.add(result);
    }
    await _save(list);
  }

  Future<void> _remove(AppFile f) async {
    if (!await confirmAction(context, 'Remove ${f.path}?', 'The file disappears from the containers with the next deploy.', 'Remove', danger: true)) return;
    await _save(List.of(_files)..remove(f));
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (_files.isEmpty)
          Text(
            'Mount files such as application.yml, a .env file or a certificate at any path in the container.',
            style: TextStyle(color: c.muted, fontSize: 13),
          ),
        for (final f in _files)
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: Icon(f.secret ? Icons.lock_rounded : Icons.description_outlined, color: f.secret ? NectarTokens.amber : c.muted),
            title: Text(f.path, style: monoStyle(context, size: 13.5)),
            subtitle: Text('${formatBytes(f.content?.length ?? f.size)}${f.secret ? ' · secret' : ''}', style: TextStyle(color: c.muted, fontSize: 12)),
            onTap: () => _edit(f),
            trailing: widget.canEdit
                ? IconButton(tooltip: 'Remove', icon: const Icon(Icons.delete_outline_rounded, size: 18), onPressed: _saving ? null : () => _remove(f))
                : null,
          ),
        if (widget.canEdit)
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton.icon(
              onPressed: _saving ? null : () => _edit(null),
              icon: const Icon(Icons.add_rounded, size: 18),
              label: const Text('Add file'),
            ),
          ),
      ],
    );
  }
}

class _FileDialog extends StatefulWidget {
  const _FileDialog({required this.file, required this.canEdit});

  final AppFile? file;
  final bool canEdit;

  @override
  State<_FileDialog> createState() => _FileDialogState();
}

class _FileDialogState extends State<_FileDialog> {
  late final _path = TextEditingController(text: widget.file?.path ?? '');
  late final _content = TextEditingController(text: widget.file?.content ?? '');
  late bool _secret = widget.file?.secret ?? false;
  late final bool _kept = widget.file != null && widget.file!.content == null; // secret not revealed
  String? _error;

  @override
  void dispose() {
    _path.dispose();
    _content.dispose();
    super.dispose();
  }

  void _ok() {
    final p = _path.text.trim();
    if (!p.startsWith('/') || p.endsWith('/') || p.contains('/../') || p.contains('//')) {
      setState(() => _error = 'Enter an absolute file path, e.g. /app/config/application.yml');
      return;
    }
    final keep = _kept && _content.text.isEmpty;
    Navigator.pop(context, AppFile(path: p, content: keep ? null : _content.text, secret: _secret));
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(widget.file == null ? 'Add a config file' : widget.file!.path),
      content: SizedBox(
        width: 640,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextField(
              controller: _path,
              enabled: widget.canEdit,
              style: monoStyle(context, size: 13.5),
              decoration: const InputDecoration(labelText: 'Path in the container', hintText: '/app/config/application.yml'),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _content,
              enabled: widget.canEdit,
              minLines: 10,
              maxLines: 18,
              style: monoStyle(context, size: 13),
              decoration: InputDecoration(
                labelText: 'Content',
                alignLabelWithHint: true,
                hintText: _kept ? 'Secret content is kept. Type here to replace it.' : null,
              ),
            ),
            const SizedBox(height: 8),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              value: _secret,
              onChanged: widget.canEdit ? (v) => setState(() => _secret = v) : null,
              title: const Text('Secret'),
              subtitle: Text('Hidden in the panel unless revealed (always encrypted at rest)', style: TextStyle(color: c.muted, fontSize: 12)),
            ),
            if (_error != null) Text(_error!, style: TextStyle(color: c.failed)),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: Text(widget.canEdit ? 'Cancel' : 'Close')),
        if (widget.canEdit) FilledButton(onPressed: _ok, child: const Text('Save file')),
      ],
    );
  }
}

/// Databases linked to the app and the variables they provide (managed on the database page).
class _LinkedDatabases extends ConsumerWidget {
  const _LinkedDatabases({required this.app});

  final App app;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final links = ref.watch(appAddonsProvider(app.id));
    final c = NectarColors.of(context);
    return NCard(
      title: 'Databases',
      trailing: TextButton(onPressed: () => context.go('/projects/${app.projectId}'), child: const Text('Project databases')),
      child: AsyncBlock(
        value: links,
        loadingHeight: 60,
        data: (list) => list.isEmpty
            ? Text('No database linked. Add one on the project page and link it to this app.', style: TextStyle(color: c.muted, fontSize: 13))
            : Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (final l in list)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      leading: Icon(l.addon.kind == 'redis' ? Icons.bolt_rounded : Icons.storage_rounded, color: NectarTokens.amber),
                      title: Text('${l.addon.name} · ${l.addon.kindName} ${l.addon.version}'),
                      subtitle: Text(l.env.join(', '), style: monoStyle(context, size: 11.5, color: c.muted)),
                      onTap: () => context.go('/addons/${l.addon.id}'),
                    ),
                  Text('Your own variables with the same names win.', style: TextStyle(color: c.muted, fontSize: 12)),
                ],
              ),
      ),
    );
  }
}
