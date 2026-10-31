import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../generated/l10n.dart';
import 'auth_frame.dart';

/// Chooses a password from a reset or invitation link (`/reset?token=…`).
class ResetPage extends ConsumerStatefulWidget {
  const ResetPage({super.key, required this.token});

  final String token;

  @override
  ConsumerState<ResetPage> createState() => _ResetPageState();
}

class _ResetPageState extends ConsumerState<ResetPage> {
  final _form = GlobalKey<FormState>();
  final _password = TextEditingController();
  final _repeat = TextEditingController();
  Future<Map<String, dynamic>>? _info;
  bool _busy = false;
  bool _done = false;
  String? _error;

  @override
  void dispose() {
    _password.dispose();
    _repeat.dispose();
    super.dispose();
  }

  Future<Map<String, dynamic>> _load(Api api) async {
    final res = await api.get('/auth/token', query: {'token': widget.token}, options: Api.public);
    return Map<String, dynamic>.from(res as Map);
  }

  Future<void> _submit(Api api, String email) async {
    if (!_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await api.post('/auth/reset', data: {'token': widget.token, 'password': _password.text}, options: Api.public);
      final session = ref.read(sessionProvider.notifier);
      if (ref.read(sessionProvider).status == SessionStatus.signedIn) await session.logout();
      final mfa = await session.login(email, _password.text);
      if (mfa != null && mounted) {
        setState(() => _done = true); // the second factor is asked on the sign-in page
      } else if (mounted) {
        context.go('/');
      }
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final api = ref.watch(sessionProvider.notifier).api;
    if (api == null) {
      return AuthFrame(title: s.authChooseAPassword, children: [Center(child: CircularProgressIndicator())]);
    }
    _info ??= _load(api);
    if (_done) {
      return AuthFrame(
        title: s.authPasswordSaved,
        children: [
          Text(s.authSignInWithYourNew),
          const SizedBox(height: 16),
          FilledButton(onPressed: () => context.go('/login'), child: Text(s.loginButton)),
        ],
      );
    }
    return FutureBuilder<Map<String, dynamic>>(
      future: _info,
      builder: (context, snap) {
        if (snap.hasError) {
          return AuthFrame(
            title: s.authLinkExpired,
            children: [
              FormError(describeError(snap.error!)),
              FilledButton(onPressed: () => context.go('/login'), child: Text(s.authGoToSignIn)),
            ],
          );
        }
        if (!snap.hasData) return AuthFrame(title: s.authChooseAPassword, children: [Center(child: CircularProgressIndicator())]);
        final info = snap.data!;
        final invite = info['purpose'] == 'invite';
        final email = '${info['email']}';
        return AuthFrame(
          title: invite ? 'Welcome${'${info['name']}'.isEmpty ? '' : ', ${info['name']}'}' : 'Choose a new password',
          subtitle: email,
          children: [
            Form(
              key: _form,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (_error != null) FormError(_error!),
                  TextFormField(
                    controller: _password,
                    autofocus: true,
                    obscureText: true,
                    autofillHints: const [AutofillHints.newPassword],
                    decoration: InputDecoration(labelText: s.loginPassword, helperText: s.authAtLeast10Characters),
                    validator: (v) => (v ?? '').length < 10 ? 'At least 10 characters' : null,
                  ),
                  const SizedBox(height: 14),
                  TextFormField(
                    controller: _repeat,
                    obscureText: true,
                    decoration: InputDecoration(labelText: s.authRepeatThePassword),
                    validator: (v) => v == _password.text ? null : 'The passwords differ',
                    onFieldSubmitted: (_) => _submit(api, email),
                  ),
                  const SizedBox(height: 22),
                  BusyButton(label: invite ? 'Create my password' : 'Save the password', busy: _busy, onPressed: () => _submit(api, email)),
                ],
              ),
            ),
          ],
        );
      },
    );
  }
}
