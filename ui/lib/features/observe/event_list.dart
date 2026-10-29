import 'package:flutter/material.dart';

import '../../api/models.dart';
import '../../core/format.dart';
import '../../theme/tokens.dart';

/// Kubernetes events, newest first: warnings stand out (crash loops, OOM kills, image pull errors).
class EventList extends StatelessWidget {
  const EventList({super.key, required this.events, this.showNamespace = false, this.empty = 'No events.'});

  final List<KubeEvent> events;
  final bool showNamespace;
  final String empty;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    if (events.isEmpty) return Text(empty, style: TextStyle(color: c.muted, fontSize: 13.5));
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final e in events)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 7),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(
                  e.isWarning ? Icons.warning_amber_rounded : Icons.info_outline_rounded,
                  size: 18,
                  color: e.isWarning ? c.warning : c.muted,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text.rich(
                        TextSpan(
                          children: [
                            TextSpan(text: e.reason, style: const TextStyle(fontWeight: FontWeight.w600)),
                            if (e.count > 1) TextSpan(text: '  ×${e.count}', style: TextStyle(color: c.muted)),
                          ],
                        ),
                        style: const TextStyle(fontSize: 13.5),
                      ),
                      Text(e.message, style: const TextStyle(fontSize: 13)),
                      Text(
                        [if (showNamespace && e.namespace.isNotEmpty) e.namespace, e.object].join(' · '),
                        style: TextStyle(color: c.muted, fontSize: 12),
                      ),
                    ],
                  ),
                ),
                const SizedBox(width: 8),
                Text(timeAgo(e.last), style: TextStyle(color: c.muted, fontSize: 12)),
              ],
            ),
          ),
      ],
    );
  }
}
