import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/copy.dart';

// ------------------------------------------------------------------- 2FA --

/// Two-factor authentication of the signed-in user.
class TwoFactorCard extends ConsumerWidget {
  const TwoFactorCard({super.key});

  Future<String?> _password(
    BuildContext context,
    String title,
    String action,
  ) async {
    final s = S.of(context);
    final ctl = TextEditingController();
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(title),
        content: SizedBox(
          width: 380,
          child: TextField(
            controller: ctl,
            autofocus: true,
            obscureText: true,
            decoration: InputDecoration(labelText: s.settingYourPassword),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(s.actionCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: Text(action),
          ),
        ],
      ),
    );
    final pw = ctl.text;
    ctl.dispose();
    return ok == true ? pw : null;
  }

  Future<void> _disable(BuildContext context, WidgetRef ref) async {
    final s = S.of(context);
    final pw = await _password(
      context,
      'Turn off two-factor authentication?',
      'Turn off',
    );
    if (pw == null || !context.mounted) return;
    try {
      await ref
          .read(apiProvider)
          .post('/me/mfa/disable', data: {'password': pw});
      ref.invalidate(mfaStatusProvider);
      if (context.mounted) {
        showSnack(context, s.settingTwoFactorAuthenticationIsOff);
      }
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _newCodes(BuildContext context, WidgetRef ref) async {
    final pw = await _password(context, 'New recovery codes', 'Create');
    if (pw == null || !context.mounted) return;
    try {
      final res = await ref
          .read(apiProvider)
          .post('/me/mfa/recovery-codes', data: {'password': pw});
      ref.invalidate(mfaStatusProvider);
      if (context.mounted) {
        await showRecoveryCodes(
          context,
          ((res as Map)['recovery_codes'] as List).cast<String>(),
        );
      }
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final status = ref.watch(mfaStatusProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: l.settingTwoFactorAuthentication,
      child: AsyncBlock(
        value: status,
        loadingHeight: 60,
        data: (s) {
          final (enabled, left) = s;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  Icon(
                    enabled
                        ? Icons.verified_user_rounded
                        : Icons.shield_outlined,
                    color: enabled ? c.running : c.muted,
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      enabled
                          ? 'On. Signing in asks for a code from your authenticator app. $left recovery codes left.'
                          : 'Off. Add a code from an authenticator app (1Password, Google Authenticator, Aegis…) to your password.',
                    ),
                  ),
                ],
              ),
              if (enabled && left <= 3)
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Text(
                    l.settingOnlyRecoveryCodesLeftCreate(left),
                    style: TextStyle(color: c.warning, fontSize: 12.5),
                  ),
                ),
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: enabled
                    ? [
                        OutlinedButton(
                          onPressed: () => _newCodes(context, ref),
                          child: Text(l.settingNewRecoveryCodes),
                        ),
                        TextButton(
                          style: TextButton.styleFrom(
                            foregroundColor: c.failed,
                          ),
                          onPressed: () => _disable(context, ref),
                          child: Text(l.settingTurnOff),
                        ),
                      ]
                    : [
                        FilledButton.icon(
                          onPressed: () => showDialog<void>(
                            context: context,
                            builder: (_) => const _SetupDialog(),
                          ),
                          icon: const Icon(Icons.qr_code_2_rounded, size: 18),
                          label: Text(l.settingSetUp),
                        ),
                      ],
              ),
            ],
          );
        },
      ),
    );
  }
}

Future<void> showRecoveryCodes(BuildContext context, List<String> codes) =>
    showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        title: Text(S.of(context).settingRecoveryCodes),
        content: SizedBox(
          width: 420,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(S.of(context).settingEachCodeSignsYouIn),
              const SizedBox(height: 14),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: NectarColors.of(ctx).elevated,
                  borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
                ),
                child: SelectableText(
                  codes.join('\n'),
                  style: monoStyle(ctx, size: 15),
                ),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () {
              copyText(ctx, codes.join('\n'), done: S.of(context).backupCopied);
            },
            child: Text(S.of(context).backupCopy),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx),
            child: Text(S.of(context).settingISavedThem),
          ),
        ],
      ),
    );

class _SetupDialog extends ConsumerStatefulWidget {
  const _SetupDialog();

  @override
  ConsumerState<_SetupDialog> createState() => _SetupDialogState();
}

class _SetupDialogState extends ConsumerState<_SetupDialog> {
  late final Future<Map<String, dynamic>> _setup = ref
      .read(apiProvider)
      .post('/me/mfa/setup')
      .then((v) => Map<String, dynamic>.from(v as Map));
  final _code = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _code.dispose();
    super.dispose();
  }

  Future<void> _enable() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final res = await ref
          .read(apiProvider)
          .post('/me/mfa/enable', data: {'code': _code.text.trim()});
      ref.invalidate(mfaStatusProvider);
      if (!mounted) return;
      Navigator.pop(context);
      await showRecoveryCodes(
        context,
        ((res as Map)['recovery_codes'] as List).cast<String>(),
      );
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
    return AlertDialog(
      title: Text(s.settingSetUpTwoFactorAuthentication),
      content: SizedBox(
        width: 420,
        child: FutureBuilder<Map<String, dynamic>>(
          future: _setup,
          builder: (context, snap) {
            if (snap.hasError) return ErrorBlock(error: snap.error!);
            if (!snap.hasData) return const LoadingBlock(height: 200);
            final url = '${snap.data!['otpauth_url']}';
            final secret = '${snap.data!['secret']}';
            return Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(s.setting1ScanThisCodeWith),
                const SizedBox(height: 12),
                Center(
                  child: Container(
                    padding: const EdgeInsets.all(10),
                    color: Colors.white,
                    child: QrImageView(
                      data: url,
                      size: 190,
                      backgroundColor: Colors.white,
                    ),
                  ),
                ),
                const SizedBox(height: 8),
                Text(
                  s.settingOrEnterTheKeyBy,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
                SelectableText(
                  secret
                      .replaceAllMapped(RegExp(r'.{4}'), (m) => '${m[0]} ')
                      .trim(),
                  style: monoStyle(context, size: 13),
                ),
                const SizedBox(height: 16),
                Text(s.setting2EnterThe6Digit),
                const SizedBox(height: 8),
                TextField(
                  controller: _code,
                  autofocus: true,
                  keyboardType: TextInputType.number,
                  inputFormatters: [
                    FilteringTextInputFormatter.digitsOnly,
                    LengthLimitingTextInputFormatter(6),
                  ],
                  decoration: InputDecoration(labelText: s.authCode),
                  onSubmitted: (_) => _enable(),
                ),
                if (_error != null)
                  Padding(
                    padding: const EdgeInsets.only(top: 8),
                    child: Text(_error!, style: TextStyle(color: c.failed)),
                  ),
              ],
            );
          },
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: _busy ? null : _enable,
          child: Text(_busy ? 'Checking…' : 'Turn on'),
        ),
      ],
    );
  }
}

// -------------------------------------------------------------- sessions --

/// Where the user is signed in, with sign-out per device.
class SessionsCard extends ConsumerWidget {
  const SessionsCard({super.key});

  Future<void> _revoke(
    BuildContext context,
    WidgetRef ref,
    UserSession s,
  ) async {
    try {
      await ref.read(apiProvider).delete('/me/sessions/${s.id}');
      ref.invalidate(mySessionsProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final sessions = ref.watch(mySessionsProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: l.settingSignedIn,
      child: AsyncBlock(
        value: sessions,
        loadingHeight: 60,
        data: (list) => Column(
          children: [
            for (final s in list)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: Icon(
                  s.kind == 'mobile'
                      ? Icons.phone_iphone_rounded
                      : Icons.laptop_rounded,
                ),
                title: Text(s.current ? '${s.device} · this device' : s.device),
                subtitle: Text(
                  l.settingActiveSince(
                    s.ip,
                    timeAgo(s.lastSeenAt),
                    timeAgo(s.createdAt),
                  ),
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
                trailing: s.current
                    ? null
                    : TextButton(
                        onPressed: () => _revoke(context, ref, s),
                        child: Text(l.logout),
                      ),
              ),
          ],
        ),
      ),
    );
  }
}

// ------------------------------------------------------------------ SMTP --

/// The panel's SMTP server (password resets, invitations, alert emails).
class SmtpCard extends ConsumerWidget {
  const SmtpCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final smtp = ref.watch(smtpSettingsProvider);
    return NCard(
      title: l.loginEmail,
      child: AsyncBlock(
        value: smtp,
        loadingHeight: 60,
        data: (s) => _SmtpForm(key: ValueKey(s), initial: s),
      ),
    );
  }
}

class _SmtpForm extends ConsumerStatefulWidget {
  const _SmtpForm({super.key, required this.initial});

  final Map<String, dynamic> initial;

  @override
  ConsumerState<_SmtpForm> createState() => _SmtpFormState();
}

class _SmtpFormState extends ConsumerState<_SmtpForm> {
  late final Map<String, TextEditingController> _c = {
    for (final k in [
      'host',
      'port',
      'username',
      'password',
      'from',
      'panel_url',
    ])
      k: TextEditingController(
        text: widget.initial[k] == null || widget.initial[k] == 0
            ? ''
            : '${widget.initial[k]}',
      ),
  };
  late String _security = '${widget.initial['security'] ?? 'starttls'}'.isEmpty
      ? 'starttls'
      : '${widget.initial['security']}';
  bool _busy = false;

  @override
  void dispose() {
    for (final c in _c.values) {
      c.dispose();
    }
    super.dispose();
  }

  Map<String, dynamic> get _body => {
    'host': _c['host']!.text.trim(),
    'port': int.tryParse(_c['port']!.text.trim()) ?? 0,
    'security': _security,
    'username': _c['username']!.text.trim(),
    'password': _c['password']!.text,
    'from': _c['from']!.text.trim(),
    'panel_url': _c['panel_url']!.text.trim(),
  };

  Future<void> _save({bool test = false}) async {
    final s = S.of(context);
    setState(() => _busy = true);
    final api = ref.read(apiProvider);
    try {
      await api.put('/settings/smtp', data: _body);
      if (test) {
        await api.post('/settings/smtp/test', data: <String, dynamic>{});
        if (mounted) showSnack(context, s.settingTestEmailSentToYour);
      } else if (mounted) {
        showSnack(context, s.settingEmailSettingsSaved);
      }
      ref.invalidate(smtpSettingsProvider);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    Widget f(
      String k,
      String label, {
      String? hint,
      double width = 260,
      bool secret = false,
    }) => SizedBox(
      width: width,
      child: TextField(
        controller: _c[k],
        obscureText: secret && _c[k]!.text != '••••••••',
        decoration: InputDecoration(
          labelText: label,
          hintText: hint,
          isDense: true,
        ),
      ),
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          s.settingUsedForPasswordResetsInvitations,
          style: TextStyle(color: c.muted, fontSize: 12.5),
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 12,
          runSpacing: 12,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            f('host', 'SMTP server', hint: 'smtp.example.com'),
            f('port', 'Port', hint: '587', width: 100),
            DropdownButton<String>(
              value: _security,
              items: [
                DropdownMenuItem(
                  value: 'starttls',
                  child: Text(s.settingStarttls),
                ),
                DropdownMenuItem(value: 'tls', child: Text(s.settingTls)),
                DropdownMenuItem(
                  value: 'none',
                  child: Text(s.settingNoEncryption),
                ),
              ],
              onChanged: (v) => setState(() => _security = v ?? 'starttls'),
            ),
            f('username', 'User name'),
            f('password', 'Password', secret: true),
            f(
              'from',
              'From',
              hint: s.settingNectarAppsPanelExampleCom,
              width: 360,
            ),
            f(
              'panel_url',
              'Panel address for links',
              hint: 'https://panel.example.com',
              width: 360,
            ),
          ],
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 8,
          children: [
            FilledButton(
              onPressed: _busy ? null : _save,
              child: Text(s.actionSave),
            ),
            OutlinedButton(
              onPressed: _busy ? null : () => _save(test: true),
              child: Text(s.settingSaveAndSendATest),
            ),
          ],
        ),
      ],
    );
  }
}

// ------------------------------------------------------------ IP allowlist --

/// Restricts which addresses may use the panel.
class AccessCard extends ConsumerWidget {
  const AccessCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final access = ref.watch(accessSettingsProvider);
    return NCard(
      title: s.settingPanelAccess,
      child: AsyncBlock(
        value: access,
        loadingHeight: 60,
        data: (a) => _AccessForm(key: ValueKey(a), initial: a),
      ),
    );
  }
}

class _AccessForm extends ConsumerStatefulWidget {
  const _AccessForm({super.key, required this.initial});

  final Map<String, dynamic> initial;

  @override
  ConsumerState<_AccessForm> createState() => _AccessFormState();
}

class _AccessFormState extends ConsumerState<_AccessForm> {
  late final _list = TextEditingController(
    text: ((widget.initial['ip_allowlist'] as List?) ?? const []).join('\n'),
  );
  bool _busy = false;

  @override
  void dispose() {
    _list.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final l = S.of(context);
    setState(() => _busy = true);
    try {
      final entries = _list.text
          .split(RegExp(r'[\s,]+'))
          .where((e) => e.isNotEmpty)
          .toList();
      await ref
          .read(apiProvider)
          .put('/settings/access', data: {'ip_allowlist': entries});
      ref.invalidate(accessSettingsProvider);
      if (mounted) {
        showSnack(
          context,
          entries.isEmpty
              ? 'Every address may use the panel'
              : l.settingOnlyTheListedAddressesMay,
        );
      }
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final ip = '${widget.initial['your_ip'] ?? ''}';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          s.settingAllowOnlyTheseIpAddresses(ip),
          style: TextStyle(color: c.muted, fontSize: 12.5),
        ),
        const SizedBox(height: 10),
        TextField(
          controller: _list,
          minLines: 3,
          maxLines: 8,
          style: monoStyle(context, size: 13),
          decoration: InputDecoration(hintText: ip),
        ),
        const SizedBox(height: 10),
        Align(
          alignment: Alignment.centerLeft,
          child: FilledButton(
            onPressed: _busy ? null : _save,
            child: Text(s.actionSave),
          ),
        ),
      ],
    );
  }
}
