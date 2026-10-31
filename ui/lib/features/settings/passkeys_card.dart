import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../core/passkeys.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// A passkey of the signed-in user (GET /me/passkeys).
class PasskeyInfo {
  const PasskeyInfo({required this.id, required this.name, required this.host, required this.synced, this.createdAt, this.lastUsedAt});

  final int id;
  final String name;
  final String host;
  final bool synced;
  final DateTime? createdAt;
  final DateTime? lastUsedAt;

  factory PasskeyInfo.fromJson(Object? j) {
    final m = j is Map ? j : const {};
    return PasskeyInfo(
      id: (m['id'] as num?)?.toInt() ?? 0,
      name: '${m['name'] ?? ''}',
      host: '${m['rp_id'] ?? ''}',
      synced: m['synced'] == true,
      createdAt: DateTime.tryParse('${m['created_at'] ?? ''}'),
      lastUsedAt: DateTime.tryParse('${m['last_used_at'] ?? ''}'),
    );
  }
}

final passkeysProvider = FutureProvider.autoDispose<List<PasskeyInfo>>((ref) async {
  final v = await ref.watch(apiProvider).get('/me/passkeys');
  return [for (final p in v is List ? v : const []) PasskeyInfo.fromJson(p)];
});

/// Settings → Passkeys: sign in with Touch ID / Face ID / Windows Hello / a phone or a security key.
class PasskeysCard extends ConsumerWidget {
  const PasskeysCard({super.key});

  Future<void> _add(BuildContext context, WidgetRef ref) async {
    final s = S.of(context);
    final name = await promptText(context, s.settingPasskeyName, s.settingPasskeyNameHint);
    if (name == null || !context.mounted) return;
    final api = ref.read(apiProvider);
    try {
      final start = await api.post('/me/passkeys/options') as Map;
      final credential = await createPasskey(Map<String, dynamic>.from(start['options'] as Map));
      await api.post('/me/passkeys', data: {'flow': start['flow'], 'name': name, 'credential': credential});
      ref.invalidate(passkeysProvider);
      if (context.mounted) showSnack(context, s.settingPasskeyAdded);
    } on PasskeyCancelled {
      // closed the browser dialog
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _delete(BuildContext context, WidgetRef ref, PasskeyInfo p) async {
    final s = S.of(context);
    if (!await confirmAction(context, s.settingPasskeyDelete(p.name), s.settingPasskeyDeleteHelp, s.actionDelete, danger: true)) return;
    try {
      await ref.read(apiProvider).delete('/me/passkeys/${p.id}');
      ref.invalidate(passkeysProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final list = ref.watch(passkeysProvider);
    final canAdd = passkeysSupported && ref.read(sessionProvider.notifier).cookieAuth;
    return NCard(
      title: s.settingPasskeys,
      trailing: canAdd
          ? TextButton.icon(onPressed: () => _add(context, ref), icon: const Icon(Icons.add_rounded, size: 18), label: Text(s.settingAddPasskey))
          : null,
      child: AsyncBlock(
        value: list,
        loadingHeight: 40,
        onRetry: () => ref.invalidate(passkeysProvider),
        data: (keys) => Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (keys.isEmpty)
              Text(s.settingPasskeysHelp, style: TextStyle(color: c.muted, fontSize: 12.5))
            else
              for (final p in keys)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  dense: true,
                  leading: Icon(p.synced ? Icons.cloud_done_outlined : Icons.key_rounded, color: c.muted, size: 20),
                  title: Text(p.name),
                  subtitle: Text(
                    [
                      p.host,
                      if (p.synced) s.settingPasskeySynced,
                      p.lastUsedAt == null ? s.settingPasskeyNeverUsed : s.settingPasskeyUsed(timeAgo(p.lastUsedAt)),
                    ].join(' · '),
                    style: TextStyle(color: c.muted, fontSize: 12),
                  ),
                  trailing: IconButton(
                    tooltip: s.actionDelete,
                    icon: Icon(Icons.delete_outline_rounded, size: 18, color: c.failed),
                    onPressed: () => _delete(context, ref, p),
                  ),
                ),
            if (!canAdd) ...[
              const SizedBox(height: 6),
              Text(s.settingPasskeysWebOnly, style: TextStyle(color: c.muted, fontSize: 12)),
            ],
          ],
        ),
      ),
    );
  }
}
