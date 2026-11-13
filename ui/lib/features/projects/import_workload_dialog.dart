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
import 'external_workloads.dart';

/// Reviews and imports a workload of an adopted namespace as a Nectar app.
Future<void> showImportWorkloadDialog(
  BuildContext context,
  int projectId,
  ExternalWorkload w,
) => showDialog<void>(
  context: context,
  builder: (_) => _ImportDialog(projectId: projectId, workload: w),
);

class _ImportDialog extends ConsumerStatefulWidget {
  const _ImportDialog({required this.projectId, required this.workload});

  final int projectId;
  final ExternalWorkload workload;

  @override
  ConsumerState<_ImportDialog> createState() => _ImportDialogState();
}

class _ImportDialogState extends ConsumerState<_ImportDialog> {
  final _slug = TextEditingController();
  Map? _plan;
  Object? _error;
  bool _busy = false;

  String get _path =>
      '/projects/${widget.projectId}/external-workloads/${widget.workload.kind.toLowerCase()}/${Uri.encodeComponent(widget.workload.name)}/import';

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _slug.dispose();
    super.dispose();
  }

  Future<void> _load([String? slug]) async {
    setState(() => _error = null);
    try {
      final plan =
          await ref
                  .read(apiProvider)
                  .get(
                    _path,
                    query: {if (slug != null && slug.isNotEmpty) 'slug': slug},
                  )
              as Map;
      if (!mounted) return;
      setState(() {
        _plan = plan;
        if (_slug.text.isEmpty) _slug.text = '${plan['slug']}';
      });
    } on Object catch (e) {
      if (mounted) setState(() => _error = e);
    }
  }

  Future<void> _import() async {
    setState(() => _busy = true);
    try {
      final res =
          await ref
                  .read(apiProvider)
                  .post(_path, data: {'slug': _slug.text.trim()})
              as Map;
      ref.invalidate(projectAppsProvider(widget.projectId));
      ref.invalidate(projectExternalProvider(widget.projectId));
      ref.invalidate(projectProvider(widget.projectId));
      final app = res['app'] as Map;
      if (!mounted) return;
      Navigator.pop(context);
      showSnack(context, S.of(context).importDone(widget.workload.name));
      context.go('/apps/${app['id']}');
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  List<String> _list(String key) => [
    for (final x in (_plan?[key] as List? ?? const [])) '$x',
  ];

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final plan = _plan;
    final problems = _list('problems');
    final warnings = _list('warnings');
    final small = TextStyle(color: c.muted, fontSize: 12.5);
    Widget row(String label, String value) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(width: 120, child: Text(label, style: small)),
          Expanded(
            child: SelectableText(value, style: monoStyle(context, size: 12.5)),
          ),
        ],
      ),
    );
    Widget notes(List<String> items, Color color, IconData icon) => Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final n in items)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(icon, size: 16, color: color),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    n,
                    style: TextStyle(fontSize: 12.5, color: color),
                  ),
                ),
              ],
            ),
          ),
      ],
    );

    Widget content;
    if (_error != null) {
      content = Text(describeError(_error!), style: TextStyle(color: c.failed));
    } else if (plan == null) {
      content = const Padding(
        padding: EdgeInsets.all(24),
        child: Center(child: CircularProgressIndicator()),
      );
    } else {
      final cfg = plan['config'] as Map? ?? const {};
      final res = cfg['resources'] as Map? ?? const {};
      final volumes = [
        for (final v in (cfg['volumes'] as List? ?? const []))
          '${(v as Map)['mount_path']} ← ${v['claim']} (${v['size_mb']} MB)',
      ];
      final domains = [
        for (final d in (plan['domains'] as List? ?? const []))
          '${(d as Map)['tls'] == true ? 'https' : 'http'}://${d['host']}${d['path_prefix'] == '/' ? '' : d['path_prefix']}',
      ];
      final ports = [
        if ((cfg['port'] as num? ?? 0) > 0) '${cfg['port']}',
        for (final p in (cfg['internal_ports'] as List? ?? const [])) '$p',
      ];
      content = SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(s.importHelp, style: small),
            const SizedBox(height: NectarTokens.gap),
            TextField(
              controller: _slug,
              decoration: InputDecoration(labelText: s.importAppSlug),
              onSubmitted: (v) => _load(v.trim()),
            ),
            const SizedBox(height: NectarTokens.gap),
            row(s.importKind, '${plan['kind']}'),
            row(s.importImage, '${cfg['image']}'),
            if (ports.isNotEmpty) row(s.importPorts, ports.join(', ')),
            row(s.importReplicas, '${cfg['replicas']}'),
            row(
              s.importResources,
              res['no_limit'] == true
                  ? s.importNoLimit
                  : '${res['cpu_millicores']}m CPU · ${res['memory_mb']} MB',
            ),
            if (_list('env').isNotEmpty)
              row(s.importEnv, _list('env').join(', ')),
            if (_list('files').isNotEmpty)
              row(s.importFiles, _list('files').join('\n')),
            if (volumes.isNotEmpty) row(s.importVolumes, volumes.join('\n')),
            if (_list('services').isNotEmpty)
              row(s.importServices, _list('services').join(', ')),
            if (domains.isNotEmpty) row(s.importDomains, domains.join('\n')),
            if (_list('hosts').isNotEmpty)
              row(s.importHosts, _list('hosts').join(', ')),
            notes(warnings, c.warning, Icons.info_outline_rounded),
            notes(problems, c.failed, Icons.block_rounded),
          ],
        ),
      );
    }
    final canImport = plan != null && problems.isEmpty && !_busy;
    return AlertDialog(
      title: Text(s.importTitle(widget.workload.name)),
      content: SizedBox(width: 560, child: content),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: canImport ? _import : null,
          child: Text(_busy ? s.importImporting : s.importAction),
        ),
      ],
    );
  }
}
