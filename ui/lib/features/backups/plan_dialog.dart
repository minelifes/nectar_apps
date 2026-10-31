import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';

/// Adds a backup plan, or edits [plan] (schedule, retention, paused).
Future<void> showPlanDialog(
  BuildContext context,
  WidgetRef ref, {
  required List<BackupTarget> targets,
  BackupPlan? plan,
}) => showDialog<void>(
  context: context,
  builder: (_) => _PlanDialog(targets: targets, plan: plan),
);

/// A thing that can be backed up.
class _Subject {
  const _Subject(this.scope, this.id, this.label);

  final String scope;
  final int id;
  final String label;

  @override
  bool operator ==(Object other) =>
      other is _Subject && other.scope == scope && other.id == id;

  @override
  int get hashCode => Object.hash(scope, id);
}

const _schedules = [
  ('0 3 * * *', 'Daily at 03:00 UTC'),
  ('0 */6 * * *', 'Every 6 hours'),
  ('0 * * * *', 'Every hour'),
  ('0 3 * * 0', 'Weekly (Sunday 03:00 UTC)'),
  ('', 'Manual only'),
];

class _PlanDialog extends ConsumerStatefulWidget {
  const _PlanDialog({required this.targets, this.plan});

  final List<BackupTarget> targets;
  final BackupPlan? plan;

  @override
  ConsumerState<_PlanDialog> createState() => _PlanDialogState();
}

class _PlanDialogState extends ConsumerState<_PlanDialog> {
  late int _target = widget.plan?.targetId ?? widget.targets.first.id;
  _Subject? _subject;
  late String _schedule = widget.plan?.schedule ?? '0 3 * * *';
  late final _custom = TextEditingController(
    text: _schedules.any((s) => s.$1 == _schedule) ? '' : _schedule,
  );
  late bool _customOn = !_schedules.any((s) => s.$1 == _schedule);
  late final _daily = TextEditingController(
    text: '${widget.plan?.keepDaily ?? 7}',
  );
  late final _weekly = TextEditingController(
    text: '${widget.plan?.keepWeekly ?? 4}',
  );
  late final _monthly = TextEditingController(
    text: '${widget.plan?.keepMonthly ?? 6}',
  );
  late bool _enabled = widget.plan?.enabled ?? true;
  late final Future<List<_Subject>> _subjects = widget.plan == null
      ? _loadSubjects()
      : Future.value(const []);
  bool _saving = false;
  String? _error;

  @override
  void dispose() {
    for (final c in [_custom, _daily, _weekly, _monthly]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<List<_Subject>> _loadSubjects() async {
    final api = ref.read(apiProvider);
    final out = <_Subject>[
      const _Subject(
        'panel',
        0,
        'Panel data (database, uploads) · needed for disaster recovery',
      ),
    ];
    final projects = listOf(await api.get('/projects'), Project.fromJson);
    for (final p in projects) {
      try {
        for (final a in listOf(
          await api.get('/projects/${p.id}/addons'),
          Addon.fromJson,
        )) {
          out.add(
            _Subject('addon', a.id, '${p.slug}/${a.slug} · ${a.kindName} dump'),
          );
        }
        for (final a in listOf(
          await api.get('/projects/${p.id}/apps'),
          App.fromJson,
        )) {
          if (a.volumes.isNotEmpty) {
            out.add(
              _Subject(
                'app',
                a.id,
                '${p.slug}/${a.slug} · volumes ${a.volumes.map((v) => v.name).join(', ')}',
              ),
            );
          }
        }
      } on Object {
        // no access to this project's apps
      }
    }
    return out;
  }

  Future<void> _save() async {
    final schedule = _customOn ? _custom.text.trim() : _schedule;
    final body = <String, dynamic>{
      'schedule': schedule,
      'keep_daily': int.tryParse(_daily.text) ?? 0,
      'keep_weekly': int.tryParse(_weekly.text) ?? 0,
      'keep_monthly': int.tryParse(_monthly.text) ?? 0,
      'enabled': _enabled,
    };
    final p = widget.plan;
    if (p == null) {
      final s = _subject;
      if (s == null) {
        setState(() => _error = 'Choose what to back up');
        return;
      }
      body.addAll({'target_id': _target, 'scope': s.scope, 'subject_id': s.id});
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final api = ref.read(apiProvider);
      if (p == null) {
        await api.post('/backup/plans', data: body);
      } else {
        await api.put('/backup/plans/${p.id}', data: body);
      }
      ref.invalidate(backupPlansProvider);
      ref.invalidate(backupTargetsProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = S.of(context);
    final c = NectarColors.of(context);
    final p = widget.plan;
    Widget keep(TextEditingController ctl, String label) => Expanded(
      child: TextField(
        controller: ctl,
        keyboardType: TextInputType.number,
        inputFormatters: [FilteringTextInputFormatter.digitsOnly],
        decoration: InputDecoration(labelText: label),
      ),
    );
    return AlertDialog(
      title: Text(p == null ? 'Add a backup plan' : 'Edit · ${p.subject}'),
      content: SizedBox(
        width: 520,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (p == null) ...[
                FutureBuilder<List<_Subject>>(
                  future: _subjects,
                  builder: (context, snap) {
                    if (!snap.hasData) return const LinearProgressIndicator();
                    return DropdownButtonFormField<_Subject>(
                      initialValue: _subject,
                      isExpanded: true,
                      decoration: InputDecoration(labelText: l.backupWhat),
                      items: [
                        for (final s in snap.data!)
                          DropdownMenuItem(
                            value: s,
                            child: Text(
                              s.label,
                              overflow: TextOverflow.ellipsis,
                            ),
                          ),
                      ],
                      onChanged: (v) => setState(() => _subject = v),
                    );
                  },
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<int>(
                  initialValue: _target,
                  decoration: InputDecoration(labelText: l.backupTo),
                  items: [
                    for (final t in widget.targets)
                      DropdownMenuItem(value: t.id, child: Text(t.name)),
                  ],
                  onChanged: (v) => setState(() => _target = v ?? _target),
                ),
                const SizedBox(height: 12),
              ],
              DropdownButtonFormField<String>(
                initialValue: _customOn ? '*' : _schedule,
                decoration: InputDecoration(labelText: l.backupWhen),
                items: [
                  for (final (v, label) in _schedules)
                    DropdownMenuItem(value: v, child: Text(label)),
                  DropdownMenuItem(value: '*', child: Text(l.backupCustomCron)),
                ],
                onChanged: (v) => setState(() {
                  _customOn = v == '*';
                  if (!_customOn) _schedule = v ?? '';
                }),
              ),
              if (_customOn)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: TextField(
                    controller: _custom,
                    decoration: InputDecoration(
                      labelText: l.backupCronExpressionUtc,
                      hintText: '30 2 * * *',
                      helperText: l.backupMinuteHourDayMonthWeekday,
                    ),
                  ),
                ),
              const SizedBox(height: 16),
              Text(l.backupKeep, style: Theme.of(context).textTheme.titleSmall),
              Row(
                children: [
                  keep(_daily, 'Daily'),
                  const SizedBox(width: 12),
                  keep(_weekly, 'Weekly'),
                  const SizedBox(width: 12),
                  keep(_monthly, 'Monthly'),
                ],
              ),
              const SizedBox(height: 6),
              Text(
                l.backupOlderSnapshotsAreRemovedAfter,
                style: TextStyle(color: c.muted, fontSize: 12),
              ),
              if (p != null)
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _enabled,
                  onChanged: (v) => setState(() => _enabled = v),
                  title: Text(l.backupScheduledBackupsOn),
                ),
              if (_error != null)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: Text(_error!, style: TextStyle(color: c.failed)),
                ),
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
          onPressed: _saving ? null : _save,
          child: Text(_saving ? 'Saving…' : 'Save'),
        ),
      ],
    );
  }
}
