import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Settings → Registries: logins for private registries (every project can pull with them) and the
/// built-in registry Git builds push to.
class RegistriesCard extends ConsumerWidget {
  const RegistriesCard({super.key});

  Future<void> _remove(
    BuildContext context,
    WidgetRef ref,
    RegistryLogin l,
  ) async {
    final s = S.of(context);
    if (!await confirmAction(
      context,
      s.settingRemoveTheLoginFor(l.host),
      s.settingAppsCanNoLongerPull,
      s.backupRemove2,
      danger: true,
    )) {
      return;
    }
    try {
      await ref.read(apiProvider).delete('/settings/registries/${l.id}');
      ref.invalidate(registriesProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final data = ref.watch(registriesProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: s.settingRegistries,
      trailing: TextButton.icon(
        onPressed: () => showDialog<void>(
          context: context,
          builder: (_) => const _LoginDialog(),
        ),
        icon: const Icon(Icons.add_rounded, size: 18),
        label: Text(s.settingAddLogin),
      ),
      child: AsyncBlock(
        value: data,
        loadingHeight: 60,
        onRetry: () => ref.invalidate(registriesProvider),
        data: (r) => Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                Icon(
                  r.builtinEnabled
                      ? Icons.check_circle_rounded
                      : Icons.info_outline_rounded,
                  size: 18,
                  color: r.builtinEnabled ? c.running : c.muted,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    r.builtinEnabled
                        ? 'Built-in registry: ${r.builtinHost} (Git builds push here; nodes pull through a k3s mirror)'
                        : 'No built-in registry: Git builds need it. Re-run install.sh with --registry on the server.',
                    style: TextStyle(
                      fontSize: 13,
                      color: r.builtinEnabled ? c.text : c.muted,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 10),
            if (r.logins.isEmpty)
              Text(
                s.settingNoPrivateRegistryLoginsPublic,
                style: TextStyle(color: c.muted, fontSize: 12.5),
              )
            else
              for (final l in r.logins)
                ListTile(
                  dense: true,
                  contentPadding: EdgeInsets.zero,
                  leading: const Icon(Icons.vpn_key_outlined, size: 20),
                  title: Text(l.host, style: monoStyle(context, size: 13)),
                  subtitle: Text(
                    s.settingChanged(l.username, timeAgo(l.updatedAt)),
                    style: TextStyle(color: c.muted, fontSize: 12),
                  ),
                  trailing: IconButton(
                    tooltip: s.backupRemove2,
                    icon: const Icon(Icons.delete_outline_rounded, size: 18),
                    onPressed: () => _remove(context, ref, l),
                  ),
                ),
            const SizedBox(height: 4),
            Text(
              s.settingLoginsAreStoredEncryptedAnd,
              style: TextStyle(color: c.muted, fontSize: 12),
            ),
          ],
        ),
      ),
    );
  }
}

class _LoginDialog extends ConsumerStatefulWidget {
  const _LoginDialog();

  @override
  ConsumerState<_LoginDialog> createState() => _LoginDialogState();
}

class _LoginDialogState extends ConsumerState<_LoginDialog> {
  final _host = TextEditingController(text: 'ghcr.io');
  final _user = TextEditingController();
  final _password = TextEditingController();
  bool _busy = false;

  @override
  void dispose() {
    _host.dispose();
    _user.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    setState(() => _busy = true);
    try {
      await ref
          .read(apiProvider)
          .put(
            '/settings/registries',
            data: {
              'host': _host.text.trim(),
              'username': _user.text.trim(),
              'password': _password.text,
            },
          );
      ref.invalidate(registriesProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return AlertDialog(
      title: Text(s.settingRegistryLogin),
      content: SizedBox(
        width: 440,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: _host,
              decoration: InputDecoration(
                labelText: s.settingRegistry,
                hintText: s.settingGhcrIoDockerIoRegistry,
              ),
            ),
            const SizedBox(height: 10),
            TextField(
              controller: _user,
              decoration: InputDecoration(labelText: s.settingUserName),
            ),
            const SizedBox(height: 10),
            TextField(
              controller: _password,
              obscureText: true,
              decoration: InputDecoration(
                labelText: s.settingPasswordOrAccessToken,
                helperText: s.settingGhcrATokenWithRead,
              ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: _busy ? null : _save,
          child: Text(s.actionSave),
        ),
      ],
    );
  }
}
