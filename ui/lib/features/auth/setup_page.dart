import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import 'auth_frame.dart';

/// First-run setup: creates the Owner account with the setup token printed by install.sh.
class SetupPage extends ConsumerStatefulWidget {
  const SetupPage({super.key, this.token});

  /// From the link `…/setup?token=…`.
  final String? token;

  @override
  ConsumerState<SetupPage> createState() => _SetupPageState();
}

class _SetupPageState extends ConsumerState<SetupPage> {
  final _form = GlobalKey<FormState>();
  late final _token = TextEditingController(text: widget.token ?? '');
  final _name = TextEditingController();
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _confirm = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    for (final c in [_token, _name, _email, _password, _confirm]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref
          .read(sessionProvider.notifier)
          .setup(
            token: _token.text,
            name: _name.text,
            email: _email.text,
            password: _password.text,
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
    final session = ref.watch(sessionProvider);
    final c = NectarColors.of(context);
    final hasToken = (widget.token ?? '').isNotEmpty;
    return AuthFrame(
      title: s.authWelcomeToNectarApps,
      subtitle:
          'Create the Owner account for ${session.serverHost.isEmpty ? 'this panel' : session.serverHost}.',
      children: [
        Form(
          key: _form,
          child: AutofillGroup(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (_error != null) FormError(_error!),
                if (!hasToken) ...[
                  TextFormField(
                    controller: _token,
                    style: monoStyle(context),
                    decoration: InputDecoration(labelText: s.authSetupToken),
                    validator: (v) => (v ?? '').trim().isEmpty
                        ? 'Paste the setup token'
                        : null,
                  ),
                  Padding(
                    padding: const EdgeInsets.only(top: 6, bottom: 14),
                    child: Text(
                      s.authTheInstallerPrintedItTogether,
                      style: TextStyle(color: c.muted, fontSize: 12.5),
                    ),
                  ),
                ],
                TextFormField(
                  controller: _name,
                  decoration: InputDecoration(labelText: s.authYourName),
                  autofillHints: const [AutofillHints.name],
                  textInputAction: TextInputAction.next,
                  validator: (v) =>
                      (v ?? '').trim().isEmpty ? 'Enter your name' : null,
                ),
                const SizedBox(height: 14),
                TextFormField(
                  controller: _email,
                  decoration: InputDecoration(labelText: s.loginEmail),
                  keyboardType: TextInputType.emailAddress,
                  autofillHints: const [AutofillHints.email],
                  textInputAction: TextInputAction.next,
                  validator: (v) =>
                      (v ?? '').contains('@') ? null : 'Enter a valid email',
                ),
                const SizedBox(height: 14),
                TextFormField(
                  controller: _password,
                  decoration: InputDecoration(
                    labelText: s.loginPassword,
                    helperText: s.authAtLeast10Characters,
                  ),
                  obscureText: true,
                  autofillHints: const [AutofillHints.newPassword],
                  textInputAction: TextInputAction.next,
                  validator: (v) => (v ?? '').length < 10
                      ? 'Use at least 10 characters'
                      : null,
                ),
                const SizedBox(height: 14),
                TextFormField(
                  controller: _confirm,
                  decoration: InputDecoration(labelText: s.authRepeatPassword),
                  obscureText: true,
                  textInputAction: TextInputAction.done,
                  onFieldSubmitted: (_) => _submit(),
                  validator: (v) =>
                      v == _password.text ? null : 'The passwords do not match',
                ),
                const SizedBox(height: 22),
                BusyButton(
                  label: s.authCreateOwnerAccount,
                  busy: _busy,
                  onPressed: _submit,
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}
