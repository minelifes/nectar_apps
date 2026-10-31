import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

final pushSettingsProvider = FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final v = await ref.watch(apiProvider).get('/settings/push');
  return v is Map ? Map<String, dynamic>.from(v) : const {};
});

/// Settings → Push notifications (optional): a Firebase project that delivers alerts to the Nectar app on
/// phones and Macs. Without it, alerts go through ntfy, Telegram, email or webhooks.
class PushCard extends ConsumerWidget {
  const PushCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final c = NectarColors.of(context);
    final data = ref.watch(pushSettingsProvider);
    return NCard(
      title: l.settingPushNotifications,
      trailing: data.hasValue
          ? TextButton(
              onPressed: () => showDialog<void>(context: context, builder: (_) => _PushDialog(current: data.value!)),
              child: Text(data.value!['settings']?['enabled'] == true ? 'Change' : 'Set up'),
            )
          : null,
      child: AsyncBlock(
        value: data,
        loadingHeight: 40,
        onRetry: () => ref.invalidate(pushSettingsProvider),
        data: (v) {
          final s = (v['settings'] as Map?) ?? const {};
          if (s['enabled'] != true) {
            return Text(
              l.settingOptionalAlertsReachTheNectar,
              style: TextStyle(color: c.muted, fontSize: 12.5),
            );
          }
          return Row(
            children: [
              Icon(Icons.check_circle_rounded, color: c.running, size: 18),
              const SizedBox(width: 8),
              Expanded(
                child: Text('On · Firebase project ${s['project_id']} · ${v['devices'] ?? 0} device(s) registered',
                    style: const TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () async {
                  try {
                    final r = await ref.read(apiProvider).post('/settings/push/test');
                    if (context.mounted) showSnack(context, 'Sent to ${(r as Map)['sent']} device(s)');
                  } on Object catch (e) {
                    if (context.mounted) showSnack(context, describeError(e), error: true);
                  }
                },
                child: Text(l.settingSendTestToMyDevices),
              ),
            ],
          );
        },
      ),
    );
  }
}

class _PushDialog extends ConsumerStatefulWidget {
  const _PushDialog({required this.current});

  final Map<String, dynamic> current;

  @override
  ConsumerState<_PushDialog> createState() => _PushDialogState();
}

class _PushDialogState extends ConsumerState<_PushDialog> {
  late final Map s = (widget.current['settings'] as Map?) ?? const {};
  late bool _enabled = s['enabled'] == true || s.isEmpty || s['project_id'] == null || s['project_id'] == '';
  late final _sender = TextEditingController(text: '${s['sender_id'] ?? ''}');
  late final _andKey = TextEditingController(text: '${s['android']?['api_key'] ?? ''}');
  late final _andApp = TextEditingController(text: '${s['android']?['app_id'] ?? ''}');
  late final _appleKey = TextEditingController(text: '${s['apple']?['api_key'] ?? ''}');
  late final _appleApp = TextEditingController(text: '${s['apple']?['app_id'] ?? ''}');
  late final _bundle = TextEditingController(text: '${s['apple']?['bundle_id'] ?? ''}');
  final _sa = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    for (final t in [_sender, _andKey, _andApp, _appleKey, _appleApp, _bundle, _sa]) {
      t.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref.read(apiProvider).put('/settings/push', data: {
        'enabled': _enabled,
        'project_id': '${s['project_id'] ?? ''}',
        'sender_id': _sender.text.trim(),
        'android': {'api_key': _andKey.text.trim(), 'app_id': _andApp.text.trim()},
        'apple': {'api_key': _appleKey.text.trim(), 'app_id': _appleApp.text.trim(), 'bundle_id': _bundle.text.trim()},
        if (_sa.text.trim().isNotEmpty) 'service_account': _sa.text.trim(),
      });
      ref.invalidate(pushSettingsProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final hasKey = widget.current['has_service_account'] == true;
    Widget section(String t) => Padding(
          padding: const EdgeInsets.only(top: 16, bottom: 4),
          child: Text(t, style: const TextStyle(fontWeight: FontWeight.w600)),
        );
    return AlertDialog(
      title: Text(s.settingPushNotifications),
      content: SizedBox(
        width: 560,
        child: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                s.setting1CreateAFirebaseProject,
                style: TextStyle(color: c.muted, fontSize: 12.5, height: 1.4),
              ),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(s.settingSendPushNotifications),
                value: _enabled,
                onChanged: (v) => setState(() => _enabled = v),
              ),
              TextField(
                controller: _sa,
                minLines: 3,
                maxLines: 6,
                style: monoStyle(context, size: 12),
                decoration: InputDecoration(
                  labelText: s.settingServiceAccountKeyJson,
                  hintText: hasKey ? 'Stored (${widget.current['service_account_email']}) — paste a new key to replace it' : '{ "type": "service_account", … }',
                  helperText: s.settingProjectSettingsServiceAccountsGenerate,
                ),
              ),
              const SizedBox(height: 10),
              TextField(controller: _sender, decoration: InputDecoration(labelText: s.settingSenderId, helperText: s.settingProjectSettingsCloudMessaging)),
              section('Android app'),
              TextField(controller: _andApp, decoration: InputDecoration(labelText: s.settingAppId, hintText: s.setting1123456789Android)),
              TextField(controller: _andKey, decoration: InputDecoration(labelText: s.settingApiKey, hintText: s.settingAiza)),
              section('Apple app (iPhone, iPad, Mac)'),
              TextField(controller: _appleApp, decoration: InputDecoration(labelText: s.settingAppId, hintText: s.setting1123456789Ios)),
              TextField(controller: _appleKey, decoration: InputDecoration(labelText: s.settingApiKey, hintText: s.settingAiza)),
              TextField(controller: _bundle, decoration: InputDecoration(labelText: s.settingBundleId, hintText: s.settingAsInXcodeNeededFor)),
              if (_error != null) Padding(padding: const EdgeInsets.only(top: 12), child: Text(_error!, style: TextStyle(color: c.failed))),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: Text(s.actionCancel)),
        FilledButton(onPressed: _busy ? null : _save, child: Text(_busy ? 'Saving…' : 'Save')),
      ],
    );
  }
}
