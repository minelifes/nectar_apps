import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import 'alert_widgets.dart';
import 'channel_dialog.dart';

/// Firing alerts, history, rules and notification channels.
class AlertsPage extends ConsumerStatefulWidget {
  const AlertsPage({super.key});

  @override
  ConsumerState<AlertsPage> createState() => _AlertsPageState();
}

class _AlertsPageState extends ConsumerState<AlertsPage> {
  String _tab = 'Firing';

  Future<void> _refresh() async {
    ref.invalidate(firingAlertsProvider);
    ref.invalidate(alertHistoryProvider);
    ref.invalidate(alertConfigProvider);
    ref.invalidate(alertChannelsProvider);
  }

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(sessionProvider);
    if (!session.can('server.view')) {
      return const PageScaffold(
        title: 'Alerts',
        children: [
          NCard(
            child: EmptyState(icon: Icons.lock_outline_rounded, title: 'No access', message: 'Your role does not include viewing the server (server.view).'),
          ),
        ],
      );
    }
    final manage = session.can('settings.manage');
    final tabs = <(String, Widget Function())>[
      ('Firing', () => const _Firing()),
      ('History', () => const _History()),
      ('Rules', () => _Rules(canEdit: manage)),
      if (manage) ('Channels', () => const _Channels()),
    ];
    final index = tabs.indexWhere((t) => t.$1 == _tab).clamp(0, tabs.length - 1);
    return PageScaffold(
      title: 'Alerts',
      subtitle: 'Get notified when apps go down, servers run out of resources or certificates are about to expire.',
      onRefresh: _refresh,
      children: [
        NTabs(index: index, labels: [for (final t in tabs) t.$1], onChanged: (i) => setState(() => _tab = tabs[i].$1)),
        const SizedBox(height: NectarTokens.gap),
        tabs[index].$2(),
      ],
    );
  }
}

class _Firing extends ConsumerWidget {
  const _Firing();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final firing = ref.watch(firingAlertsProvider);
    final channels = ref.watch(sessionProvider.select((s) => s.can('settings.manage'))) ? ref.watch(alertChannelsProvider) : null;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (channels != null && channels.hasValue && channels.value!.where((c) => c.enabled).isEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: NectarTokens.gap),
            child: NCard(
              child: Row(
                children: [
                  Icon(Icons.notifications_paused_outlined, color: NectarColors.of(context).warning),
                  const SizedBox(width: 12),
                  const Expanded(child: Text('No notification channel yet: alerts only show up here. Add ntfy, Telegram, email or a webhook under Channels.')),
                ],
              ),
            ),
          ),
        NCard(
          child: AsyncBlock(
            value: firing,
            onRetry: () => ref.invalidate(firingAlertsProvider),
            data: (list) => list.isEmpty
                ? noAlerts('Nothing is firing. Rules are checked every 30 seconds.')
                : Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [for (final a in list) AlertTile(alert: a)]),
          ),
        ),
      ],
    );
  }
}

class _History extends ConsumerWidget {
  const _History();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final history = ref.watch(alertHistoryProvider);
    return NCard(
      child: AsyncBlock(
        value: history,
        onRetry: () => ref.invalidate(alertHistoryProvider),
        data: (list) => list.isEmpty
            ? noAlerts('No alert has fired yet.')
            : Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (var i = 0; i < list.length; i++) ...[
                    if (i > 0) const Divider(height: 1),
                    AlertTile(alert: list[i]),
                  ],
                ],
              ),
      ),
    );
  }
}

// -------------------------------------------------------------------- rules --

class _Rules extends ConsumerWidget {
  const _Rules({required this.canEdit});

  final bool canEdit;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final cfg = ref.watch(alertConfigProvider);
    return AsyncBlock(
      value: cfg,
      onRetry: () => ref.invalidate(alertConfigProvider),
      data: (c) => _RulesEditor(key: ValueKey(c), config: c, canEdit: canEdit),
    );
  }
}

class _RulesEditor extends ConsumerStatefulWidget {
  const _RulesEditor({super.key, required this.config, required this.canEdit});

  final AlertConfig config;
  final bool canEdit;

  @override
  ConsumerState<_RulesEditor> createState() => _RulesEditorState();
}

const _forOptions = [0, 30, 60, 120, 300, 600, 900, 1800, 3600];
const _repeatOptions = [0, 1, 3, 6, 12, 24];

class _RulesEditorState extends ConsumerState<_RulesEditor> {
  late List<AlertRule> _rules = List.of(widget.config.rules);
  late final _panelUrl = TextEditingController(text: widget.config.panelUrl);
  late int _repeat = widget.config.repeatHours;
  late final Map<String, TextEditingController> _thresholds = {
    for (final r in widget.config.rules) r.kind: TextEditingController(text: _num(r.threshold)),
  };
  bool _saving = false;
  bool _dirty = false;

  static String _num(double v) => v == v.roundToDouble() ? v.toInt().toString() : v.toString();

  @override
  void dispose() {
    _panelUrl.dispose();
    for (final c in _thresholds.values) {
      c.dispose();
    }
    super.dispose();
  }

  void _set(int i, AlertRule r) => setState(() {
        _rules = List.of(_rules)..[i] = r;
        _dirty = true;
      });

  Future<void> _save() async {
    final rules = <Map<String, dynamic>>[];
    for (final r in _rules) {
      final def = widget.config.def(r.kind);
      var threshold = r.threshold;
      if (def != null && def.unit.isNotEmpty) {
        final v = double.tryParse(_thresholds[r.kind]!.text.trim().replaceAll(',', '.'));
        if (v == null) {
          showSnack(context, '${def.title}: enter a number', error: true);
          return;
        }
        threshold = v;
      }
      rules.add(r.copyWith(threshold: threshold).toJson());
    }
    setState(() => _saving = true);
    try {
      await ref.read(apiProvider).put('/alerts/rules', data: {'rules': rules, 'panel_url': _panelUrl.text.trim(), 'repeat_hours': _repeat});
      ref.invalidate(alertConfigProvider);
      if (mounted) showSnack(context, 'Alert rules saved');
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final edit = widget.canEdit;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        NCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (var i = 0; i < _rules.length; i++) ...[
                if (i > 0) const Divider(height: 24),
                _ruleRow(context, i),
              ],
            ],
          ),
        ),
        const SizedBox(height: NectarTokens.gap),
        NCard(
          title: 'Notifications',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              TextField(
                controller: _panelUrl,
                enabled: edit,
                decoration: const InputDecoration(
                  labelText: 'Panel address for links',
                  hintText: 'https://panel.example.com',
                  helperText: 'Notifications link to the affected app. Leave empty for no links.',
                ),
                onChanged: (_) => setState(() => _dirty = true),
              ),
              const SizedBox(height: 16),
              Row(
                children: [
                  const Expanded(child: Text('Remind me while an alert keeps firing')),
                  DropdownButton<int>(
                    value: _repeatOptions.contains(_repeat) ? _repeat : 0,
                    items: [
                      for (final h in _repeatOptions)
                        DropdownMenuItem(value: h, child: Text(h == 0 ? 'Never' : (h == 1 ? 'Every hour' : 'Every $h hours'))),
                    ],
                    onChanged: edit ? (v) => setState(() {
                          _repeat = v ?? 0;
                          _dirty = true;
                        }) : null,
                  ),
                ],
              ),
            ],
          ),
        ),
        if (edit) ...[
          const SizedBox(height: NectarTokens.gap),
          Align(
            alignment: Alignment.centerRight,
            child: FilledButton(
              onPressed: _saving || !_dirty ? null : _save,
              child: Text(_saving ? 'Saving…' : 'Save rules'),
            ),
          ),
        ] else
          Padding(
            padding: const EdgeInsets.only(top: 12),
            child: Text('Changing rules needs the settings.manage permission.', style: TextStyle(color: c.muted, fontSize: 12.5)),
          ),
      ],
    );
  }

  Widget _ruleRow(BuildContext context, int i) {
    final c = NectarColors.of(context);
    final r = _rules[i];
    final def = widget.config.def(r.kind);
    final edit = widget.canEdit;
    final title = def?.title ?? r.kind;
    return Wrap(
      spacing: 16,
      runSpacing: 10,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        SizedBox(
          width: 360,
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Switch(value: r.enabled, onChanged: edit ? (v) => _set(i, r.copyWith(enabled: v)) : null),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Flexible(child: Text(title, style: const TextStyle(fontWeight: FontWeight.w600))),
                        const SizedBox(width: 8),
                        Text(def?.severity ?? '', style: TextStyle(fontSize: 11.5, color: def?.severity == 'critical' ? c.failed : c.warning)),
                      ],
                    ),
                    if (def != null) Text(def.description, style: TextStyle(color: c.muted, fontSize: 12.5)),
                  ],
                ),
              ),
            ],
          ),
        ),
        if (def != null && def.unit.isNotEmpty)
          SizedBox(
            width: 130,
            child: TextField(
              controller: _thresholds[r.kind],
              enabled: edit && r.enabled,
              keyboardType: const TextInputType.numberWithOptions(decimal: true),
              decoration: InputDecoration(isDense: true, labelText: def.unit == '%' ? 'Above' : 'Less than', suffixText: def.unit),
              onChanged: (_) => setState(() => _dirty = true),
            ),
          ),
        if (def == null || (def.kind != 'cert_expiry' && !def.event))
          SizedBox(
            width: 170,
            child: DropdownButtonFormField<int>(
              initialValue: _forOptions.contains(r.forSeconds) ? r.forSeconds : null,
              isDense: true,
              decoration: const InputDecoration(isDense: true, labelText: 'For at least'),
              items: [for (final s in _forOptions) DropdownMenuItem(value: s, child: Text(formatFor(s)))],
              onChanged: edit && r.enabled ? (v) => _set(i, r.copyWith(forSeconds: v ?? 0)) : null,
            ),
          ),
      ],
    );
  }
}

// ----------------------------------------------------------------- channels --

class _Channels extends ConsumerWidget {
  const _Channels();

  Future<void> _test(BuildContext context, WidgetRef ref, AlertChannel ch) async {
    try {
      await ref.read(apiProvider).post('/alerts/channels/${ch.id}/test');
      if (context.mounted) showSnack(context, 'Test message sent to ${ch.name}');
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _toggle(BuildContext context, WidgetRef ref, AlertChannel ch, bool enabled) async {
    try {
      await ref.read(apiProvider).put('/alerts/channels/${ch.id}', data: {'name': ch.name, 'enabled': enabled, 'config': ch.config});
      ref.invalidate(alertChannelsProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _delete(BuildContext context, WidgetRef ref, AlertChannel ch) async {
    if (!await confirmAction(context, 'Delete ${ch.name}?', 'Alerts will no longer be sent there.', 'Delete', danger: true)) return;
    try {
      await ref.read(apiProvider).delete('/alerts/channels/${ch.id}');
      ref.invalidate(alertChannelsProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final channels = ref.watch(alertChannelsProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: 'Channels',
      trailing: FilledButton.icon(
        onPressed: () => showChannelDialog(context, ref),
        icon: const Icon(Icons.add_rounded, size: 18),
        label: const Text('Add channel'),
      ),
      child: AsyncBlock(
        value: channels,
        onRetry: () => ref.invalidate(alertChannelsProvider),
        data: (list) {
          if (list.isEmpty) {
            return const EmptyState(
              icon: Icons.campaign_outlined,
              title: 'No channels yet',
              message: 'ntfy is the quickest: install the ntfy app, subscribe to a topic name only you know, and add it here.',
            );
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (var i = 0; i < list.length; i++) ...[
                if (i > 0) const Divider(height: 1),
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 8),
                  child: Row(
                    children: [
                      Icon(channelIcon(list[i].kind), color: list[i].enabled ? NectarTokens.amber : c.muted),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(list[i].name, style: const TextStyle(fontWeight: FontWeight.w600)),
                            Text('${channelLabel(list[i].kind)} · ${list[i].summary}',
                                maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: c.muted, fontSize: 12.5)),
                          ],
                        ),
                      ),
                      Switch(value: list[i].enabled, onChanged: (v) => _toggle(context, ref, list[i], v)),
                      IconButton(tooltip: 'Send a test', icon: const Icon(Icons.send_rounded, size: 18), onPressed: () => _test(context, ref, list[i])),
                      IconButton(
                        tooltip: 'Edit',
                        icon: const Icon(Icons.edit_outlined, size: 18),
                        onPressed: () => showChannelDialog(context, ref, channel: list[i]),
                      ),
                      IconButton(tooltip: 'Delete', icon: const Icon(Icons.delete_outline_rounded, size: 18), onPressed: () => _delete(context, ref, list[i])),
                    ],
                  ),
                ),
              ],
            ],
          );
        },
      ),
    );
  }
}
