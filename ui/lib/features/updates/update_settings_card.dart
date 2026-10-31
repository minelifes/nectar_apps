import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

const _policies = {
  'off': 'Off',
  'notify': 'Notify only',
  'patch': 'Auto: patch releases (+ security fixes)',
  'minor': 'Auto: patch + minor releases',
};
const _k3sPolicies = {
  'off': 'Off',
  'notify': 'Notify only',
  'auto': 'Auto: follow the channel',
};
const _days = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

/// Policies per component, the maintenance window and the check interval.
class UpdateSettingsCard extends ConsumerStatefulWidget {
  const UpdateSettingsCard({super.key, required this.settings});

  final Map<String, dynamic> settings;

  @override
  ConsumerState<UpdateSettingsCard> createState() => _UpdateSettingsCardState();
}

class _UpdateSettingsCardState extends ConsumerState<UpdateSettingsCard> {
  late Map<String, Map<String, dynamic>> _comp;
  late Set<String> _windowDays;
  late final _start = TextEditingController();
  late final _end = TextEditingController();
  late final _tz = TextEditingController();
  late final _panelFeed = TextEditingController();
  late final _gatewayFeed = TextEditingController();
  late final _k3sChannel = TextEditingController();
  int _interval = 6;
  bool _saving = false;
  bool _dirty = false;

  @override
  void initState() {
    super.initState();
    _reset(widget.settings);
  }

  @override
  void didUpdateWidget(UpdateSettingsCard old) {
    super.didUpdateWidget(old);
    if (!_dirty && old.settings != widget.settings) _reset(widget.settings);
  }

  void _reset(Map<String, dynamic> s) {
    Map<String, dynamic> m(String k) =>
        Map<String, dynamic>.from(s[k] as Map? ?? const {});
    _comp = {'panel': m('panel'), 'gateway': m('gateway'), 'k3s': m('k3s')};
    final w = Map<String, dynamic>.from(s['window'] as Map? ?? const {});
    _windowDays = {for (final d in w['days'] as List? ?? const []) '$d'};
    _start.text = '${w['start'] ?? ''}';
    _end.text = '${w['end'] ?? ''}';
    _tz.text = '${w['timezone'] ?? ''}';
    _panelFeed.text = '${_comp['panel']!['feed_url'] ?? ''}';
    _gatewayFeed.text = '${_comp['gateway']!['feed_url'] ?? ''}';
    _k3sChannel.text = '${_comp['k3s']!['channel'] ?? 'stable'}';
    _interval = (s['interval_hours'] as num?)?.toInt() ?? 6;
  }

  @override
  void dispose() {
    for (final c in [
      _start,
      _end,
      _tz,
      _panelFeed,
      _gatewayFeed,
      _k3sChannel,
    ]) {
      c.dispose();
    }
    super.dispose();
  }

  void _set(String comp, String key, Object value) => setState(() {
    _comp[comp]![key] = value;
    _dirty = true;
  });

  Future<void> _save() async {
    final s = S.of(context);
    setState(() => _saving = true);
    final body = {
      'panel': {..._comp['panel']!, 'feed_url': _panelFeed.text.trim()},
      'gateway': {..._comp['gateway']!, 'feed_url': _gatewayFeed.text.trim()},
      'k3s': {
        ..._comp['k3s']!,
        'channel': _k3sChannel.text.trim().isEmpty
            ? 'stable'
            : _k3sChannel.text.trim(),
      },
      'window': {
        'days': [
          for (final d in _days)
            if (_windowDays.contains(d)) d,
        ],
        'start': _start.text.trim(),
        'end': _end.text.trim(),
        'timezone': _tz.text.trim(),
      },
      'interval_hours': _interval,
    };
    try {
      await ref.read(apiProvider).put('/updates/settings', data: body);
      _dirty = false;
      ref.invalidate(updatesProvider);
      if (mounted) showSnack(context, s.updateUpdateSettingsSaved);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Widget _policyRow(
    String comp,
    String label,
    Map<String, String> policies, {
    bool beta = true,
  }) {
    final l = S.of(context);
    final m = _comp[comp]!;
    final policy = policies.containsKey(m['policy'])
        ? '${m['policy']}'
        : policies.keys.first;
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Wrap(
        spacing: 12,
        runSpacing: 8,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          SizedBox(
            width: 150,
            child: Text(
              label,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
          ),
          SizedBox(
            width: 320,
            child: DropdownButtonFormField<String>(
              key: ValueKey('$comp-$policy'),
              initialValue: policy,
              isExpanded: true,
              decoration: InputDecoration(
                labelText: l.updatePolicy,
                isDense: true,
              ),
              items: [
                for (final e in policies.entries)
                  DropdownMenuItem(value: e.key, child: Text(e.value)),
              ],
              onChanged: (v) {
                if (v != null) _set(comp, 'policy', v);
              },
            ),
          ),
          if (beta)
            SegmentedButton<String>(
              segments: [
                ButtonSegment(value: 'stable', label: Text(l.updateStable)),
                ButtonSegment(value: 'beta', label: Text(l.updateBeta)),
              ],
              selected: {m['channel'] == 'beta' ? 'beta' : 'stable'},
              onSelectionChanged: (s) => _set(comp, 'channel', s.first),
            )
          else
            SizedBox(
              width: 160,
              child: TextField(
                controller: _k3sChannel,
                decoration: InputDecoration(
                  labelText: l.updateChannel,
                  hintText: l.updateStableLatestV131,
                  isDense: true,
                ),
                onChanged: (_) => _dirty = true,
              ),
            ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return NCard(
      title: s.updateAutomaticUpdates,
      trailing: FilledButton(
        onPressed: _saving ? null : _save,
        child: Text(_saving ? 'Saving…' : 'Save'),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _policyRow('panel', 'Panel', _policies),
          _policyRow('gateway', 'Gateway', _policies),
          _policyRow('k3s', 'k3s', _k3sPolicies, beta: false),
          Text(
            s.updateMajorAndBreakingReleasesAre,
            style: TextStyle(color: c.muted, fontSize: 12.5),
          ),
          const SizedBox(height: 18),
          const SectionLabel('Maintenance window'),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              for (final d in _days)
                FilterChip(
                  label: Text(d[0].toUpperCase() + d.substring(1)),
                  selected: _windowDays.contains(d),
                  onSelected: (on) => setState(() {
                    on ? _windowDays.add(d) : _windowDays.remove(d);
                    _dirty = true;
                  }),
                ),
            ],
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 12,
            runSpacing: 8,
            children: [
              SizedBox(
                width: 110,
                child: TextField(
                  controller: _start,
                  decoration: InputDecoration(
                    labelText: s.updateFrom,
                    hintText: '03:00',
                    isDense: true,
                  ),
                  onChanged: (_) => _dirty = true,
                ),
              ),
              SizedBox(
                width: 110,
                child: TextField(
                  controller: _end,
                  decoration: InputDecoration(
                    labelText: s.backupTo,
                    hintText: '05:00',
                    isDense: true,
                  ),
                  onChanged: (_) => _dirty = true,
                ),
              ),
              SizedBox(
                width: 220,
                child: TextField(
                  controller: _tz,
                  decoration: InputDecoration(
                    labelText: s.updateTimeZone,
                    hintText: s.updateUtcEuropeKyiv,
                    isDense: true,
                  ),
                  onChanged: (_) => _dirty = true,
                ),
              ),
              SizedBox(
                width: 180,
                child: DropdownButtonFormField<int>(
                  initialValue: const [1, 3, 6, 12, 24].contains(_interval)
                      ? _interval
                      : 6,
                  decoration: InputDecoration(
                    labelText: s.updateCheckEvery,
                    isDense: true,
                  ),
                  items: [
                    for (final h in const [1, 3, 6, 12, 24])
                      DropdownMenuItem(
                        value: h,
                        child: Text(h == 1 ? '1 hour' : '$h hours'),
                      ),
                  ],
                  onChanged: (v) => setState(() {
                    _interval = v ?? 6;
                    _dirty = true;
                  }),
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          Text(
            s.updateNoDaysAndNoTimes,
            style: TextStyle(color: c.muted, fontSize: 12.5),
          ),
          ExpansionTile(
            tilePadding: EdgeInsets.zero,
            title: Text(s.updateUpdateSources, style: TextStyle(fontSize: 14)),
            childrenPadding: const EdgeInsets.only(bottom: 8),
            children: [
              TextField(
                controller: _panelFeed,
                decoration: InputDecoration(
                  labelText: s.updatePanelFeedUrl,
                  hintText:
                      'https://minelifes.github.io/nectar_apps/updates.json',
                  isDense: true,
                ),
                onChanged: (_) => _dirty = true,
              ),
              const SizedBox(height: 10),
              TextField(
                controller: _gatewayFeed,
                decoration: InputDecoration(
                  labelText: s.updateGatewayFeedUrl,
                  hintText:
                      'https://minelifes.github.io/nectar_gateway/updates.json',
                  isDense: true,
                ),
                onChanged: (_) => _dirty = true,
              ),
              const SizedBox(height: 6),
              Text(
                s.updateEmptyTheOfficialFeedsWhen,
                style: TextStyle(color: c.muted, fontSize: 12.5),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
