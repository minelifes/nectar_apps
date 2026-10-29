import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/models.dart';
import '../../theme/tokens.dart';
import 'app_actions.dart';

/// − n + replicas, applied live (no new release).
class ScaleControl extends ConsumerStatefulWidget {
  const ScaleControl({super.key, required this.app});

  final App app;

  @override
  ConsumerState<ScaleControl> createState() => _ScaleControlState();
}

class _ScaleControlState extends ConsumerState<ScaleControl> {
  bool _busy = false;

  Future<void> _scale(int n) async {
    setState(() => _busy = true);
    await appOperation(context, ref, widget.app, 'scale', data: {'replicas': n}, done: n == 0 ? '${widget.app.name} stopped' : 'Scaled to $n');
    if (mounted) setState(() => _busy = false);
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final n = widget.app.replicas;
    return Container(
      decoration: BoxDecoration(border: Border.all(color: c.border), borderRadius: BorderRadius.circular(NectarTokens.radiusSmall)),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          IconButton(
            tooltip: n == 1 ? 'Stop' : 'Fewer replicas',
            visualDensity: VisualDensity.compact,
            icon: const Icon(Icons.remove_rounded, size: 16),
            onPressed: _busy || n == 0 ? null : () => _scale(n - 1),
          ),
          SizedBox(width: 22, child: Text('$n', textAlign: TextAlign.center, style: const TextStyle(fontWeight: FontWeight.w600))),
          IconButton(
            tooltip: 'More replicas',
            visualDensity: VisualDensity.compact,
            icon: const Icon(Icons.add_rounded, size: 16),
            onPressed: _busy || n >= 20 ? null : () => _scale(n + 1),
          ),
        ],
      ),
    );
  }
}

/// Tells that saved settings, image or environment changes are not live yet, with a redeploy button.
class PendingBanner extends ConsumerWidget {
  const PendingBanner({super.key, required this.app, required this.pending, required this.canDeploy, required this.onStarted});

  final App app;
  final List<String> pending;
  final bool canDeploy;
  final void Function(Release?) onStarted;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = NectarColors.of(context);
    final what = [
      if (pending.contains('settings')) 'settings',
      if (pending.contains('image')) 'version',
      if (pending.contains('environment')) 'environment variables or files',
    ];
    final list = what.length <= 1 ? what.join() : '${what.sublist(0, what.length - 1).join(', ')} and ${what.last}';
    return Padding(
      padding: const EdgeInsets.only(bottom: NectarTokens.gap),
      child: Container(
        padding: const EdgeInsets.fromLTRB(14, 10, 10, 10),
        decoration: BoxDecoration(
          color: NectarTokens.amber.withValues(alpha: 0.08),
          border: Border.all(color: NectarTokens.amber.withValues(alpha: 0.5)),
          borderRadius: BorderRadius.circular(NectarTokens.radius),
        ),
        child: Row(
          children: [
            const Icon(Icons.pending_actions_rounded, color: NectarTokens.amber, size: 20),
            const SizedBox(width: 12),
            Expanded(child: Text('Changed $list are saved but not live yet.', style: TextStyle(color: c.text, fontSize: 13.5))),
            if (canDeploy)
              FilledButton(
                onPressed: () async => onStarted(await redeploy(context, ref, app)),
                child: const Text('Redeploy'),
              ),
          ],
        ),
      ),
    );
  }
}
