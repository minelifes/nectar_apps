import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Edits the labels people set on a node (PUT /server/nodes/{name}/labels). Kubernetes', k3s' and
/// Nectar's own labels are kept and not shown. Apps pick nodes by these labels (Settings → Placement).
Future<void> showNodeLabelsDialog(BuildContext context, NodeInfo node) =>
    showDialog<void>(
      context: context,
      builder: (_) => _NodeLabelsDialog(node: node),
    );

class _Row {
  _Row(String k, String v)
    : key = TextEditingController(text: k),
      value = TextEditingController(text: v);
  final TextEditingController key;
  final TextEditingController value;
  void dispose() {
    key.dispose();
    value.dispose();
  }
}

class _NodeLabelsDialog extends ConsumerStatefulWidget {
  const _NodeLabelsDialog({required this.node});
  final NodeInfo node;

  @override
  ConsumerState<_NodeLabelsDialog> createState() => _NodeLabelsDialogState();
}

class _NodeLabelsDialogState extends ConsumerState<_NodeLabelsDialog> {
  late final List<_Row> _rows = [
    for (final e
        in (widget.node.userLabels.entries.toList()
          ..sort((a, b) => a.key.compareTo(b.key))))
      _Row(e.key, e.value),
  ];
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    if (_rows.isEmpty) _rows.add(_Row('', ''));
  }

  @override
  void dispose() {
    for (final r in _rows) {
      r.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    final s = S.of(context);
    final labels = <String, String>{};
    for (final r in _rows) {
      final k = r.key.text.trim();
      if (k.isEmpty) continue;
      if (!isEditableNodeLabel(k)) {
        showSnack(context, s.nodeLabelsSystemKey(k), error: true);
        return;
      }
      labels[k] = r.value.text.trim();
    }
    setState(() => _saving = true);
    try {
      await ref
          .read(apiProvider)
          .put(
            '/server/nodes/${widget.node.name}/labels',
            data: {'labels': labels},
          );
      ref.invalidate(nodesProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(s.nodeLabelsTitle(widget.node.name)),
      content: SizedBox(
        width: 520,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                s.nodeLabelsHelp,
                style: TextStyle(color: c.muted, fontSize: 13),
              ),
              const SizedBox(height: NectarTokens.gap),
              for (final r in _rows)
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          controller: r.key,
                          style: monoStyle(context, size: 13),
                          decoration: InputDecoration(
                            labelText: s.nodeLabelKey,
                            hintText: 'disk',
                            isDense: true,
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: TextField(
                          controller: r.value,
                          style: monoStyle(context, size: 13),
                          decoration: InputDecoration(
                            labelText: s.nodeLabelValue,
                            hintText: 'ssd',
                            isDense: true,
                          ),
                        ),
                      ),
                      IconButton(
                        tooltip: s.nodeLabelRemove,
                        icon: const Icon(Icons.close_rounded, size: 18),
                        onPressed: () => setState(() {
                          _rows.remove(r);
                          r.dispose();
                        }),
                      ),
                    ],
                  ),
                ),
              TextButton.icon(
                onPressed: () => setState(() => _rows.add(_Row('', ''))),
                icon: const Icon(Icons.add_rounded, size: 18),
                label: Text(s.nodeLabelAdd),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _saving ? null : () => Navigator.pop(context),
          child: Text(s.nodeLabelCancel),
        ),
        FilledButton(
          onPressed: _saving ? null : _save,
          child: Text(s.nodeLabelSave),
        ),
      ],
    );
  }
}
