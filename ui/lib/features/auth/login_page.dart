import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../core/passkeys.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import 'auth_frame.dart';

class LoginPage extends ConsumerStatefulWidget {
  const LoginPage({super.key});

  @override
  ConsumerState<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends ConsumerState<LoginPage> {
  final _form = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _code = TextEditingController();
  String? _mfaToken; // set after the password when the account uses two-factor authentication
  bool _busy = false;
  bool _obscure = true;
  String? _error;
  List<({String id, String name, String kind})> _sso = const [];

  @override
  void initState() {
    super.initState();
    // a failed single sign-on comes back as /login?sso_error=…
    final ssoError = Uri.base.queryParameters['sso_error'];
    if (ssoError != null && ssoError.isNotEmpty) _error = 'Single sign-on failed: $ssoError';
    _loadSSO();
  }

  Future<void> _loadSSO() async {
    final session = ref.read(sessionProvider.notifier);
    final api = session.api;
    if (api == null || !session.cookieAuth) return;
    try {
      final list = await api.get('/auth/sso', options: Api.public);
      if (!mounted || list is! List) return;
      setState(() => _sso = [
            for (final p in list.whereType<Map>()) (id: '${p['id']}', name: '${p['name']}', kind: '${p['kind']}'),
          ]);
    } on Object {
      // an older panel without single sign-on: password only
    }
  }

  void _startSSO(String id) {
    final api = ref.read(sessionProvider.notifier).api;
    if (api == null) return;
    setState(() => _busy = true);
    launchUrl(Uri.parse('${api.baseUrl}/api/auth/sso/$id/start?next=/'), webOnlyWindowName: '_self');
  }

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    _code.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final session = ref.read(sessionProvider.notifier);
      final token = _mfaToken;
      if (token != null) {
        await session.loginMfa(token, _code.text);
      } else {
        final mfa = await session.login(_email.text, _password.text);
        if (mfa != null) {
          if (mounted) setState(() => _mfaToken = mfa);
          return;
        }
      }
      TextInput.finishAutofillContext();
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// Passkeys work in the web panel (cookie sessions) of browsers with the WebAuthn JSON helpers.
  bool get _passkeys => passkeysSupported && ref.read(sessionProvider.notifier).cookieAuth;

  Future<void> _passkey() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref.read(sessionProvider.notifier).loginWithPasskey();
    } on PasskeyCancelled {
      // the browser dialog was closed: nothing to say
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _forgot() async {
    final s = S.of(context);
    final email = TextEditingController(text: _email.text);
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(s.authResetYourPassword),
        content: SizedBox(
          width: 380,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(s.authWeEmailYouALink),
              const SizedBox(height: 12),
              TextField(controller: email, autofocus: true, keyboardType: TextInputType.emailAddress, decoration: InputDecoration(labelText: s.loginEmail)),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: Text(s.actionCancel)),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: Text(s.authSendLink)),
        ],
      ),
    );
    final address = email.text;
    email.dispose();
    if (ok != true || !mounted) return;
    try {
      await ref.read(sessionProvider.notifier).forgotPassword(address);
      if (mounted) setState(() => _error = null);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s.authIfTheAddressHasAn)));
      }
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = ref.watch(sessionProvider);
    final c = NectarColors.of(context);

    if (session.status == SessionStatus.unreachable) {
      return AuthFrame(
        title: s.authCannotReachThePanel,
        subtitle: session.serverHost,
        footer: session.canChangeServer
            ? TextButton(onPressed: () => ref.read(sessionProvider.notifier).disconnect(), child: Text(s.authUseAnotherServer))
            : null,
        children: [
          FormError(session.error ?? 'The server did not answer.'),
          FilledButton.icon(
            onPressed: () => ref.read(sessionProvider.notifier).bootstrap(),
            icon: const Icon(Icons.refresh_rounded, size: 18),
            label: Text(s.actionRetry),
          ),
        ],
      );
    }

    final notice = _error ?? session.error;
    return AuthFrame(
      title: s.loginButton,
      subtitle: session.serverHost.isEmpty ? null : session.serverHost,
      footer: session.canChangeServer
          ? TextButton(onPressed: () => ref.read(sessionProvider.notifier).disconnect(), child: Text(s.authUseAnotherServer))
          : Text(
              session.version == null ? '' : 'Nectar Apps ${session.version!.version}',
              textAlign: TextAlign.center,
              style: TextStyle(color: c.muted, fontSize: 12),
            ),
      children: [
        Form(
          key: _form,
          child: AutofillGroup(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (notice != null) FormError(notice),
                if (_mfaToken != null) ...[
                  Text(s.authEnterThe6DigitCode, style: TextStyle(color: c.muted)),
                  const SizedBox(height: 14),
                  TextFormField(
                    controller: _code,
                    autofocus: true,
                    decoration: InputDecoration(labelText: s.authCode),
                    keyboardType: TextInputType.visiblePassword,
                    autofillHints: const [AutofillHints.oneTimeCode],
                    onFieldSubmitted: (_) => _submit(),
                    validator: (v) => (v ?? '').trim().length < 6 ? 'Enter the code' : null,
                  ),
                  const SizedBox(height: 22),
                  BusyButton(label: s.authVerify, busy: _busy, onPressed: _submit),
                  TextButton(
                    onPressed: () => setState(() {
                      _mfaToken = null;
                      _code.clear();
                    }),
                    child: Text(s.projectBack),
                  ),
                ] else ...[
                TextFormField(
                  controller: _email,
                  decoration: InputDecoration(labelText: s.loginEmail),
                  keyboardType: TextInputType.emailAddress,
                  autofillHints: const [AutofillHints.email, AutofillHints.username],
                  textInputAction: TextInputAction.next,
                  autofocus: true,
                  validator: (v) => (v ?? '').contains('@') ? null : 'Enter your email',
                ),
                const SizedBox(height: 14),
                TextFormField(
                  controller: _password,
                  decoration: InputDecoration(
                    labelText: s.loginPassword,
                    suffixIcon: IconButton(
                      tooltip: _obscure ? 'Show password' : 'Hide password',
                      icon: Icon(_obscure ? Icons.visibility_outlined : Icons.visibility_off_outlined, size: 20),
                      onPressed: () => setState(() => _obscure = !_obscure),
                    ),
                  ),
                  obscureText: _obscure,
                  autofillHints: const [AutofillHints.password],
                  textInputAction: TextInputAction.done,
                  onFieldSubmitted: (_) => _submit(),
                  validator: (v) => (v ?? '').isEmpty ? 'Enter your password' : null,
                ),
                const SizedBox(height: 22),
                BusyButton(label: s.loginButton, busy: _busy, onPressed: _submit),
                TextButton(onPressed: _forgot, child: Text(s.authForgotYourPassword)),
                if (_sso.isNotEmpty || _passkeys) ...[
                  Padding(
                    padding: const EdgeInsets.symmetric(vertical: 10),
                    child: Row(children: [
                      const Expanded(child: Divider()),
                      Padding(padding: const EdgeInsets.symmetric(horizontal: 10), child: Text(s.authOr, style: TextStyle(color: c.muted, fontSize: 12.5))),
                      const Expanded(child: Divider()),
                    ]),
                  ),
                  for (final p in _sso)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 8),
                      child: OutlinedButton.icon(
                        onPressed: _busy ? null : () => _startSSO(p.id),
                        icon: Icon(switch (p.kind) { 'github' => Icons.code_rounded, 'google' => Icons.g_mobiledata_rounded, _ => Icons.badge_outlined }, size: 20),
                        label: Text(s.authContinueWith(p.name)),
                      ),
                    ),
                  if (_passkeys)
                    OutlinedButton.icon(
                      onPressed: _busy ? null : _passkey,
                      icon: const Icon(Icons.key_rounded, size: 20),
                      label: Text(s.authSignInWithPasskey),
                    ),
                ],
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }
}
