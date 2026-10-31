import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Server → Add node: the command that joins another machine to the cluster as a worker.
class AddNodeDialog extends ConsumerStatefulWidget {
  const AddNodeDialog({super.key});

  @override
  ConsumerState<AddNodeDialog> createState() => _AddNodeDialogState();
}

class _AddNodeDialogState extends ConsumerState<AddNodeDialog> {
  late final Future<Map<String, dynamic>> _info = ref
      .read(apiProvider)
      .get('/server/join')
      .then((v) => Map<String, dynamic>.from(v as Map));

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(s.serverAddANode),
      content: SizedBox(
        width: 680,
        child: FutureBuilder<Map<String, dynamic>>(
          future: _info,
          builder: (context, snap) {
            if (snap.hasError) {
              return Text(
                describeError(snap.error!),
                style: TextStyle(color: c.failed),
              );
            }
            if (!snap.hasData) {
              return const SizedBox(
                height: 120,
                child: Center(child: CircularProgressIndicator()),
              );
            }
            final cmd = '${snap.data!['command']}';
            final reqs = [
              for (final r in snap.data!['requirements'] as List? ?? const [])
                '$r',
            ];
            return Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(s.serverRunThisAsRootOn),
                const SizedBox(height: 10),
                Container(
                  padding: const EdgeInsets.all(10),
                  decoration: BoxDecoration(
                    color: const Color(0xFF0A0B0D),
                    borderRadius: BorderRadius.circular(
                      NectarTokens.radiusSmall,
                    ),
                  ),
                  child: SelectableText(
                    cmd,
                    style: monoStyle(
                      context,
                      size: 12,
                      color: const Color(0xFFD9DBE0),
                    ),
                  ),
                ),
                const SizedBox(height: 12),
                for (final r in reqs)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 4),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Icon(Icons.check_rounded, size: 16, color: c.muted),
                        const SizedBox(width: 6),
                        Expanded(
                          child: Text(r, style: const TextStyle(fontSize: 13)),
                        ),
                      ],
                    ),
                  ),
                const SizedBox(height: 6),
                Text(
                  s.serverTheCommandContainsTheCluster,
                  style: TextStyle(color: c.warning, fontSize: 12.5),
                ),
              ],
            );
          },
        ),
      ),
      actions: [
        FutureBuilder<Map<String, dynamic>>(
          future: _info,
          builder: (context, snap) => TextButton(
            onPressed: snap.hasData
                ? () {
                    Clipboard.setData(
                      ClipboardData(text: '${snap.data!['command']}'),
                    );
                    showSnack(context, s.backupCopied);
                  }
                : null,
            child: Text(s.serverCopyCommand),
          ),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.backupDone),
        ),
      ],
    );
  }
}
