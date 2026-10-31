import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import 'updates_page.dart';

Future<void> showUpdateDialog(
  BuildContext context,
  WidgetRef ref, {
  required String component,
  required ComponentUpdate state,
}) => showDialog<void>(
  context: context,
  builder: (_) => _UpdateDialog(component: component, state: state),
);

/// Pick a version, see the plan (both components when needed), the release notes and the warnings, then start.
class _UpdateDialog extends ConsumerStatefulWidget {
  const _UpdateDialog({required this.component, required this.state});

  final String component;
  final ComponentUpdate state;

  @override
  ConsumerState<_UpdateDialog> createState() => _UpdateDialogState();
}

class _UpdateDialogState extends ConsumerState<_UpdateDialog> {
  late String _version = widget.state.available.first.version;
  Map<String, dynamic>? _plan;
  Object? _error;
  bool _loading = true;
  bool _starting = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final res = await ref
          .read(apiProvider)
          .post('/updates/plan', data: {widget.component: _version});
      if (mounted) {
        setState(() => _plan = Map<String, dynamic>.from(res as Map));
      }
    } on Object catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _start() async {
    final s = S.of(context);
    setState(() => _starting = true);
    try {
      await ref
          .read(apiProvider)
          .post('/updates/apply', data: {widget.component: _version});
      ref.read(updateInProgressProvider.notifier).set(true);
      ref.invalidate(updatesProvider);
      if (mounted) {
        Navigator.pop(context);
        showSnack(context, s.updateUpdateStarted);
      }
    } on Object catch (e) {
      if (mounted) {
        setState(() => _starting = false);
        showSnack(context, describeError(e), error: true);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = S.of(context);
    final c = NectarColors.of(context);
    final plan = _plan;
    final steps = plan == null
        ? const <UpdateStep>[]
        : [
            for (final s
                in (plan['plan'] as Map?)?['steps'] as List? ?? const [])
              UpdateStep.fromJson(s),
          ];
    final notes = plan == null
        ? const <String>[]
        : [
            for (final n
                in (plan['plan'] as Map?)?['notes'] as List? ?? const [])
              '$n',
          ];
    final warnings = plan == null
        ? const <String>[]
        : [for (final w in plan['warnings'] as List? ?? const []) '$w'];
    final releases = plan == null
        ? const <ReleaseInfo>[]
        : [
            for (final r in plan['releases'] as List? ?? const [])
              ReleaseInfo.fromJson(r),
          ];
    return AlertDialog(
      title: Text(
        l.updateUpdate(componentNames[widget.component] ?? widget.component),
      ),
      content: SizedBox(
        width: 620,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              DropdownButtonFormField<String>(
                initialValue: _version,
                decoration: InputDecoration(labelText: l.updateVersion),
                items: [
                  for (final r in widget.state.available)
                    DropdownMenuItem(
                      value: r.version,
                      child: Text(
                        '${r.version}  ·  ${r.kind}${r.security ? ' · security' : ''}${r.breaking ? ' · breaking' : ''}',
                      ),
                    ),
                ],
                onChanged: _loading
                    ? null
                    : (v) {
                        if (v == null) return;
                        setState(() => _version = v);
                        _load();
                      },
              ),
              const SizedBox(height: 14),
              if (_loading)
                const Padding(
                  padding: EdgeInsets.all(20),
                  child: Center(child: CircularProgressIndicator()),
                )
              else if (_error != null)
                Text(describeError(_error!), style: TextStyle(color: c.failed))
              else ...[
                const SectionLabel('What happens'),
                for (final s in steps) RunStepRow(step: s),
                for (final n in notes)
                  Text(n, style: TextStyle(color: c.muted, fontSize: 12.5)),
                const SizedBox(height: 6),
                Text(
                  l.updateEachStepTheDatabaseIs,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
                if (warnings.isNotEmpty) ...[
                  const SizedBox(height: 12),
                  for (final w in warnings)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 4),
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Icon(
                            Icons.info_outline_rounded,
                            size: 16,
                            color: c.warning,
                          ),
                          const SizedBox(width: 6),
                          Expanded(
                            child: Text(
                              w,
                              style: const TextStyle(fontSize: 13),
                            ),
                          ),
                        ],
                      ),
                    ),
                ],
                if (releases.isNotEmpty) ...[
                  const SizedBox(height: 12),
                  const SectionLabel('Release notes'),
                  for (final r in releases)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            '${componentNames[r.component] ?? r.component} ${r.version}',
                            style: const TextStyle(fontWeight: FontWeight.w600),
                          ),
                          const SizedBox(height: 2),
                          Container(
                            width: double.infinity,
                            padding: const EdgeInsets.all(8),
                            decoration: BoxDecoration(
                              color: c.elevated,
                              borderRadius: BorderRadius.circular(
                                NectarTokens.radiusSmall,
                              ),
                            ),
                            child: SelectableText(
                              r.notes.isEmpty ? 'No notes.' : r.notes,
                              style: monoStyle(context, size: 12),
                            ),
                          ),
                        ],
                      ),
                    ),
                ],
              ],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(l.actionCancel),
        ),
        FilledButton(
          onPressed: _loading || _error != null || _starting || steps.isEmpty
              ? null
              : _start,
          child: Text(_starting ? 'Starting…' : 'Update now'),
        ),
      ],
    );
  }
}
