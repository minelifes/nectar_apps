import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
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

  Future<void> _forgot() async {
    final email = TextEditingController(text: _email.text);
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Reset your password'),
        content: SizedBox(
          width: 380,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text('We email you a link to choose a new password (if the panel can send emails).'),
              const SizedBox(height: 12),
              TextField(controller: email, autofocus: true, keyboardType: TextInputType.emailAddress, decoration: const InputDecoration(labelText: 'Email')),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Send link')),
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
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('If the address has an account, a reset link is on its way.')));
      }
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    }
  }

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(sessionProvider);
    final c = NectarColors.of(context);

    if (session.status == SessionStatus.unreachable) {
      return AuthFrame(
        title: 'Cannot reach the panel',
        subtitle: session.serverHost,
        footer: session.canChangeServer
            ? TextButton(onPressed: () => ref.read(sessionProvider.notifier).disconnect(), child: const Text('Use another server'))
            : null,
        children: [
          FormError(session.error ?? 'The server did not answer.'),
          FilledButton.icon(
            onPressed: () => ref.read(sessionProvider.notifier).bootstrap(),
            icon: const Icon(Icons.refresh_rounded, size: 18),
            label: const Text('Try again'),
          ),
        ],
      );
    }

    final notice = _error ?? session.error;
    return AuthFrame(
      title: 'Sign in',
      subtitle: session.serverHost.isEmpty ? null : session.serverHost,
      footer: session.canChangeServer
          ? TextButton(onPressed: () => ref.read(sessionProvider.notifier).disconnect(), child: const Text('Use another server'))
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
                  Text('Enter the 6-digit code from your authenticator app, or one of your recovery codes.', style: TextStyle(color: c.muted)),
                  const SizedBox(height: 14),
                  TextFormField(
                    controller: _code,
                    autofocus: true,
                    decoration: const InputDecoration(labelText: 'Code'),
                    keyboardType: TextInputType.visiblePassword,
                    autofillHints: const [AutofillHints.oneTimeCode],
                    onFieldSubmitted: (_) => _submit(),
                    validator: (v) => (v ?? '').trim().length < 6 ? 'Enter the code' : null,
                  ),
                  const SizedBox(height: 22),
                  BusyButton(label: 'Verify', busy: _busy, onPressed: _submit),
                  TextButton(
                    onPressed: () => setState(() {
                      _mfaToken = null;
                      _code.clear();
                    }),
                    child: const Text('Back'),
                  ),
                ] else ...[
                TextFormField(
                  controller: _email,
                  decoration: const InputDecoration(labelText: 'Email'),
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
                    labelText: 'Password',
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
                BusyButton(label: 'Sign in', busy: _busy, onPressed: _submit),
                TextButton(onPressed: _forgot, child: const Text('Forgot your password?')),
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }
}
