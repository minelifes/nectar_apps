import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// A one-click app from GET /api/templates.
class AppTemplate {
  const AppTemplate({
    required this.id,
    required this.name,
    required this.category,
    required this.description,
    required this.docs,
    required this.notes,
    required this.inputs,
    required this.images,
    required this.addons,
  });

  final String id;
  final String name;
  final String category;
  final String description;
  final String docs;
  final String notes;
  final List<({String key, String label, String def, bool required})> inputs;
  final List<String> images;
  final List<String> addons;

  factory AppTemplate.fromJson(Map m) => AppTemplate(
        id: '${m['id']}',
        name: '${m['name']}',
        category: '${m['category'] ?? ''}',
        description: '${m['description'] ?? ''}',
        docs: '${m['docs'] ?? ''}',
        notes: '${m['notes'] ?? ''}',
        inputs: [
          for (final i in (m['inputs'] as List? ?? const []))
            (key: '${(i as Map)['key']}', label: '${i['label'] ?? i['key']}', def: '${i['default'] ?? ''}', required: i['required'] == true),
        ],
        images: [for (final i in (m['images'] as List? ?? const [])) '$i'],
        addons: [for (final i in (m['addons'] as List? ?? const [])) '$i'],
      );
}

final templatesProvider = FutureProvider.autoDispose<List<AppTemplate>>((ref) async {
  final res = await ref.watch(apiProvider).get('/templates');
  return [for (final t in (res as List)) AppTemplate.fromJson(t as Map)];
});

IconData _templateIcon(String category) => switch (category) {
      'Websites' => Icons.public_rounded,
      'Monitoring' => Icons.monitor_heart_outlined,
      'Automation' => Icons.account_tree_outlined,
      'Storage' => Icons.inventory_2_outlined,
      'Databases' => Icons.storage_rounded,
      'Messaging' => Icons.forum_outlined,
      'Developer tools' => Icons.code_rounded,
      _ => Icons.apps_rounded,
    };

/// The one-click app gallery: pick a template, name it, install it into the project.
Future<void> showTemplateGallery(BuildContext context, int projectId) =>
    showDialog<void>(context: context, builder: (_) => _TemplateGallery(projectId: projectId));

class _TemplateGallery extends ConsumerStatefulWidget {
  const _TemplateGallery({required this.projectId});

  final int projectId;

  @override
  ConsumerState<_TemplateGallery> createState() => _TemplateGalleryState();
}

class _TemplateGalleryState extends ConsumerState<_TemplateGallery> {
  AppTemplate? _chosen;
  final _name = TextEditingController();
  final Map<String, TextEditingController> _inputs = {};
  bool _busy = false;
  List<Map>? _steps;
  String _notes = '';
  int _appId = 0;

  @override
  void dispose() {
    _name.dispose();
    for (final c in _inputs.values) {
      c.dispose();
    }
    super.dispose();
  }

  void _choose(AppTemplate t) {
    setState(() {
      _chosen = t;
      _name.text = t.id;
      for (final i in t.inputs) {
        _inputs[i.key] ??= TextEditingController(text: i.def);
      }
    });
  }

  Future<void> _install() async {
    final t = _chosen!;
    setState(() => _busy = true);
    try {
      final res = await ref.read(apiProvider).post('/projects/${widget.projectId}/templates/${t.id}', data: {
        'name': _name.text.trim(),
        'inputs': {for (final i in t.inputs) i.key: _inputs[i.key]!.text.trim()},
      }) as Map;
      ref.invalidate(projectAppsProvider(widget.projectId));
      ref.invalidate(projectAddonsProvider(widget.projectId));
      setState(() {
        _steps = [for (final s in (res['steps'] as List? ?? const [])) s as Map];
        _notes = '${res['notes'] ?? ''}';
        _appId = (res['app_id'] as num?)?.toInt() ?? 0;
      });
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final t = _chosen;
    return AlertDialog(
      title: Text(t == null ? 'One-click apps' : (_steps == null ? 'Install ${t.name}' : '${t.name} installed')),
      content: SizedBox(
        width: 640,
        child: _steps != null
            ? SingleChildScrollView(child: ApplyReport(steps: _steps!, notes: _notes.isEmpty ? const [] : [_notes]))
            : t == null
                ? _gallery(context, c)
                : _form(context, c, t),
      ),
      actions: [
        if (t != null && _steps == null) TextButton(onPressed: _busy ? null : () => setState(() => _chosen = null), child: const Text('Back')),
        TextButton(onPressed: () => Navigator.pop(context), child: Text(_steps == null ? 'Cancel' : 'Close')),
        if (t != null && _steps == null) FilledButton(onPressed: _busy ? null : _install, child: Text(_busy ? 'Installing…' : 'Install')),
        if (_steps != null && _appId > 0)
          FilledButton(
            onPressed: () {
              Navigator.pop(context);
              context.go('/apps/$_appId');
            },
            child: const Text('Open the app'),
          ),
      ],
    );
  }

  Widget _gallery(BuildContext context, NectarColors c) {
    final list = ref.watch(templatesProvider);
    return SizedBox(
      height: 460,
      child: AsyncBlock(
        value: list,
        onRetry: () => ref.invalidate(templatesProvider),
        data: (ts) => GridView.extent(
          maxCrossAxisExtent: 300,
          mainAxisSpacing: 10,
          crossAxisSpacing: 10,
          childAspectRatio: 2.2,
          children: [
            for (final t in ts)
              NCard(
                onTap: () => _choose(t),
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(children: [
                      Icon(_templateIcon(t.category), size: 18, color: NectarTokens.amber),
                      const SizedBox(width: 8),
                      Expanded(child: Text(t.name, style: Theme.of(context).textTheme.titleSmall)),
                      Text(t.category, style: TextStyle(color: c.muted, fontSize: 11)),
                    ]),
                    const SizedBox(height: 6),
                    Text(t.description, maxLines: 2, overflow: TextOverflow.ellipsis, style: TextStyle(color: c.muted, fontSize: 12.5)),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _form(BuildContext context, NectarColors c, AppTemplate t) => SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(t.description),
            const SizedBox(height: 8),
            Text(
              [
                'Images: ${t.images.join(', ')}',
                if (t.addons.isNotEmpty) 'Database: ${t.addons.join(', ')} (managed add-on)',
              ].join('\n'),
              style: monoStyle(context, size: 12, color: c.muted),
            ),
            const SizedBox(height: 14),
            TextField(
              controller: _name,
              decoration: const InputDecoration(labelText: 'Name', helperText: 'The app slug; its default domain is <name>-<project>.<base domain>'),
            ),
            for (final i in t.inputs) ...[
              const SizedBox(height: 10),
              TextField(controller: _inputs[i.key], decoration: InputDecoration(labelText: i.label + (i.required ? ' *' : ''))),
            ],
            const SizedBox(height: 12),
            Text('Passwords are generated and stored as secret variables. The app is deployed right away.', style: TextStyle(color: c.muted, fontSize: 12.5)),
          ],
        ),
      );
}

/// The steps of an apply / import / install.
class ApplyReport extends StatelessWidget {
  const ApplyReport({super.key, required this.steps, this.notes = const []});

  final List<Map> steps;
  final List<String> notes;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        for (final s in steps)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 2),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(
                  switch ('${s['action']}') {
                    'failed' => Icons.error_outline_rounded,
                    'skipped' => Icons.remove_circle_outline_rounded,
                    'exists' => Icons.check_circle_outline_rounded,
                    _ => Icons.check_circle_rounded,
                  },
                  size: 16,
                  color: switch ('${s['action']}') {
                    'failed' => c.failed,
                    'skipped' || 'exists' => c.muted,
                    _ => c.running,
                  },
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text.rich(TextSpan(children: [
                    TextSpan(text: '${s['action']} ${s['kind']} '),
                    TextSpan(text: '${s['name']}', style: monoStyle(context, size: 13)),
                    if ('${s['message'] ?? ''}'.isNotEmpty) TextSpan(text: ' — ${s['message']}', style: TextStyle(color: c.muted)),
                  ])),
                ),
              ],
            ),
          ),
        for (final n in notes) ...[
          const SizedBox(height: 8),
          Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Icon(Icons.info_outline_rounded, size: 16, color: c.warning),
            const SizedBox(width: 8),
            Expanded(child: SelectableText(n, style: const TextStyle(fontSize: 13))),
          ]),
        ],
      ],
    );
  }
}

/// Import a docker-compose.yml or a nectar.yaml into the project: paste or choose the file, preview, apply.
Future<void> showImportDialog(BuildContext context, int projectId, {bool compose = true}) =>
    showDialog<void>(context: context, builder: (_) => _ImportDialog(projectId: projectId, compose: compose));

class _ImportDialog extends ConsumerStatefulWidget {
  const _ImportDialog({required this.projectId, required this.compose});

  final int projectId;
  final bool compose;

  @override
  ConsumerState<_ImportDialog> createState() => _ImportDialogState();
}

class _ImportDialogState extends ConsumerState<_ImportDialog> {
  final _text = TextEditingController();
  bool _deploy = true;
  bool _busy = false;
  String? _preview;
  List<String> _notes = const [];
  List<Map>? _steps;

  String get _path => widget.compose ? '/projects/${widget.projectId}/import/compose' : '/projects/${widget.projectId}/apply';
  String get _key => widget.compose ? 'compose' : 'blueprint';

  @override
  void dispose() {
    _text.dispose();
    super.dispose();
  }

  Future<void> _pick() async {
    final res = await FilePicker.platform.pickFiles(withData: true, type: FileType.custom, allowedExtensions: ['yml', 'yaml', 'json']);
    final f = res?.files.single;
    if (f?.bytes != null) setState(() => _text.text = utf8.decode(f!.bytes!, allowMalformed: true));
  }

  Future<void> _run({required bool dryRun}) async {
    setState(() => _busy = true);
    try {
      final res = await ref.read(apiProvider).post(_path, data: {_key: _text.text, 'deploy': _deploy, 'dry_run': dryRun}) as Map;
      setState(() {
        _notes = [for (final n in (res['notes'] as List? ?? const [])) '$n'];
        if (dryRun) {
          _preview = '${res['yaml'] ?? 'The file is valid.'}';
        } else {
          _steps = [for (final s in (res['steps'] as List? ?? const [])) s as Map];
          ref.invalidate(projectAppsProvider(widget.projectId));
          ref.invalidate(projectAddonsProvider(widget.projectId));
        }
      });
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final what = widget.compose ? 'docker-compose.yml' : 'nectar.yaml';
    return AlertDialog(
      title: Text('Import $what'),
      content: SizedBox(
        width: 720,
        child: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              if (_steps != null)
                ApplyReport(steps: _steps!, notes: _notes)
              else ...[
                Text(
                  widget.compose
                      ? 'Every service becomes an app with the same name, so the services still find each other. Services with published ports get a domain; databases stay internal. Nothing is deleted.'
                      : 'Creates the apps, databases, links and domains that are missing and updates the existing apps. Nothing is deleted; secret variables without a value keep theirs.',
                  style: TextStyle(color: c.muted, fontSize: 13),
                ),
                const SizedBox(height: 10),
                Row(children: [
                  OutlinedButton.icon(onPressed: _pick, icon: const Icon(Icons.file_open_outlined, size: 18), label: Text('Choose $what')),
                  const SizedBox(width: 12),
                  Text('or paste it below', style: TextStyle(color: c.muted, fontSize: 12.5)),
                ]),
                const SizedBox(height: 10),
                TextField(
                  controller: _text,
                  minLines: 10,
                  maxLines: 18,
                  style: monoStyle(context, size: 12.5),
                  decoration: const InputDecoration(border: OutlineInputBorder()),
                  onChanged: (_) => setState(() => _preview = null),
                ),
                CheckboxListTile(
                  contentPadding: EdgeInsets.zero,
                  controlAffinity: ListTileControlAffinity.leading,
                  value: _deploy,
                  onChanged: (v) => setState(() => _deploy = v ?? true),
                  title: const Text('Deploy the apps'),
                ),
                if (_preview != null) ...[
                  const SectionLabel('Preview'),
                  Container(
                    padding: const EdgeInsets.all(10),
                    decoration: BoxDecoration(border: Border.all(color: c.border), borderRadius: BorderRadius.circular(NectarTokens.radius)),
                    child: SelectableText(_preview!, style: monoStyle(context, size: 12)),
                  ),
                  if (_notes.isNotEmpty) ApplyReport(steps: const [], notes: _notes),
                ],
              ],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: Text(_steps == null ? 'Cancel' : 'Close')),
        if (_steps == null) ...[
          OutlinedButton(onPressed: _busy || _text.text.trim().isEmpty ? null : () => _run(dryRun: true), child: const Text('Check')),
          FilledButton(onPressed: _busy || _text.text.trim().isEmpty ? null : () => _run(dryRun: false), child: Text(_busy ? 'Importing…' : 'Import')),
        ],
      ],
    );
  }
}

/// Shows the project's nectar.yaml with copy (and the CLI command to apply it).
Future<void> showExportDialog(BuildContext context, WidgetRef ref, int projectId, String slug) async {
  try {
    final yaml = await ref.read(apiProvider).get('/projects/$projectId/export', options: Options(responseType: ResponseType.plain));
    if (!context.mounted) return;
    await showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('nectar.yaml'),
        content: SizedBox(
          width: 720,
          height: 480,
          child: SingleChildScrollView(child: SelectableText('$yaml', style: monoStyle(ctx, size: 12))),
        ),
        actions: [
          TextButton(
            onPressed: () {
              Clipboard.setData(ClipboardData(text: '$yaml'));
              showSnack(ctx, 'Copied — apply it with: nectar apply -f nectar.yaml --project $slug');
            },
            child: const Text('Copy'),
          ),
          FilledButton(onPressed: () => Navigator.pop(ctx), child: const Text('Close')),
        ],
      ),
    );
  } on Object catch (e) {
    if (context.mounted) showSnack(context, describeError(e), error: true);
  }
}
