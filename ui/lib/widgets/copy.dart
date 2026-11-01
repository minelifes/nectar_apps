import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../generated/l10n.dart';
import '../theme/theme.dart';
import '../theme/tokens.dart';
import 'common.dart';

/// Browsers only let a page write to the clipboard on HTTPS (or localhost). The panel is often opened at
/// `http://<server-ip>:30480`, where Clipboard.setData does nothing.
bool get clipboardBlocked {
  if (!kIsWeb) return false;
  final u = Uri.base;
  return u.scheme != 'https' &&
      !const {'localhost', '127.0.0.1', '::1', '[::1]'}.contains(u.host);
}

/// Copies [text] and confirms with [done] (default "Copied"; nothing when [quiet]). Where the browser does
/// not allow it, a dialog shows the text selected, so Ctrl+C / ⌘C copies it (a normal text selection).
Future<void> copyText(
  BuildContext context,
  String text, {
  String? done,
  bool quiet = false,
}) async {
  var ok = !clipboardBlocked;
  if (ok) {
    try {
      await Clipboard.setData(ClipboardData(text: text));
    } on Object {
      ok = false;
    }
  }
  if (!context.mounted) return;
  final s = S.of(context);
  if (ok) {
    if (!quiet) showSnack(context, done ?? s.backupCopied);
    return;
  }
  await showDialog<void>(
    context: context,
    builder: (_) => _CopyDialog(text: text),
  );
}

class _CopyDialog extends StatefulWidget {
  const _CopyDialog({required this.text});

  final String text;

  @override
  State<_CopyDialog> createState() => _CopyDialogState();
}

class _CopyDialogState extends State<_CopyDialog> {
  late final _ctl = TextEditingController(
    text: widget.text,
  )..selection = TextSelection(baseOffset: 0, extentOffset: widget.text.length);

  @override
  void dispose() {
    _ctl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(s.copyPressToCopy),
      content: SizedBox(
        width: 560,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              s.copyBrowserBlocksClipboard,
              style: TextStyle(color: c.muted, fontSize: 12.5),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _ctl,
              readOnly: true,
              autofocus: true,
              minLines: 1,
              maxLines: 8,
              style: monoStyle(context, size: 12.5),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionClose),
        ),
      ],
    );
  }
}
