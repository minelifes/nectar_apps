import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import 'auth_frame.dart';

/// Native apps: the address of the user's panel.
class ConnectPage extends ConsumerStatefulWidget {
  const ConnectPage({super.key});

  @override
  ConsumerState<ConnectPage> createState() => _ConnectPageState();
}

class _ConnectPageState extends ConsumerState<ConnectPage> {
  final _url = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _url.dispose();
    super.dispose();
  }

  Future<void> _connect() async {
    if (_url.text.trim().isEmpty) {
      setState(() => _error = 'Enter the address of your panel.');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref.read(sessionProvider.notifier).connect(_url.text);
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
    return AuthFrame(
      title: s.authConnectToYourServer,
      subtitle: s.authNectarAppsRunsOnYour,
      children: [
        if (_error != null) FormError(_error!),
        TextField(
          controller: _url,
          style: monoStyle(context, size: 14),
          decoration: InputDecoration(
            labelText: s.authPanelAddress,
            hintText: 'panel.apps.example.com',
          ),
          keyboardType: TextInputType.url,
          autocorrect: false,
          autofocus: true,
          textInputAction: TextInputAction.go,
          onSubmitted: (_) => _connect(),
        ),
        Padding(
          padding: const EdgeInsets.only(top: 8, bottom: 20),
          child: Text(
            s.authBeforeThePanelHasA,
            style: TextStyle(color: c.muted, fontSize: 12.5),
          ),
        ),
        BusyButton(label: s.authConnect, busy: _busy, onPressed: _connect),
      ],
    );
  }
}
