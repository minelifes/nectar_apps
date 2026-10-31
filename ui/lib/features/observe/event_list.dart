import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/models.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Card with the Kubernetes events of an app or a database ([path]: `/apps/5` or `/addons/3`).
class EventsCard extends ConsumerWidget {
  const EventsCard({super.key, required this.path});

  final String path;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final events = ref.watch(eventsProvider(path));
    return NCard(
      title: s.appKubernetesEvents,
      trailing: IconButton(
        tooltip: s.actionRefresh,
        icon: const Icon(Icons.refresh_rounded, size: 18),
        onPressed: () => ref.invalidate(eventsProvider(path)),
      ),
      child: AsyncBlock(
        value: events,
        loadingHeight: 100,
        onRetry: () => ref.invalidate(eventsProvider(path)),
        data: (list) => EventList(
          events: list,
          empty: 'No recent events. Kubernetes keeps them for about an hour.',
        ),
      ),
    );
  }
}

/// Kubernetes events, newest first: warnings stand out (crash loops, OOM kills, image pull errors).
class EventList extends StatelessWidget {
  const EventList({
    super.key,
    required this.events,
    this.showNamespace = false,
    this.empty = 'No events.',
  });

  final List<KubeEvent> events;
  final bool showNamespace;
  final String empty;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    if (events.isEmpty) {
      return Text(empty, style: TextStyle(color: c.muted, fontSize: 13.5));
    }
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
                  e.isWarning
                      ? Icons.warning_amber_rounded
                      : Icons.info_outline_rounded,
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
                            TextSpan(
                              text: e.reason,
                              style: const TextStyle(
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                            if (e.count > 1)
                              TextSpan(
                                text: '  ×${e.count}',
                                style: TextStyle(color: c.muted),
                              ),
                          ],
                        ),
                        style: const TextStyle(fontSize: 13.5),
                      ),
                      Text(e.message, style: const TextStyle(fontSize: 13)),
                      Text(
                        [
                          if (showNamespace && e.namespace.isNotEmpty)
                            e.namespace,
                          e.object,
                        ].join(' · '),
                        style: TextStyle(color: c.muted, fontSize: 12),
                      ),
                    ],
                  ),
                ),
                const SizedBox(width: 8),
                Text(
                  timeAgo(e.last),
                  style: TextStyle(color: c.muted, fontSize: 12),
                ),
              ],
            ),
          ),
      ],
    );
  }
}
