import 'package:flutter/material.dart';

import '../api/models.dart';
import '../core/format.dart';
import '../generated/l10n.dart';
import '../theme/tokens.dart';
import 'common.dart';

/// Who did what, newest first (audit log entries).
class ActivityCard extends StatelessWidget {
  const ActivityCard({
    super.key,
    required this.entries,
    this.title = 'Recent activity',
  });

  final List<AuditEntry> entries;
  final String title;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return NCard(
      title: title,
      child: entries.isEmpty
          ? Text(s.widgetNothingYet, style: TextStyle(color: c.muted))
          : Column(
              children: [
                for (final e in entries)
                  Padding(
                    padding: const EdgeInsets.symmetric(vertical: 7),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Padding(
                          padding: const EdgeInsets.only(top: 6),
                          child: Container(
                            width: 6,
                            height: 6,
                            decoration: const BoxDecoration(
                              color: NectarTokens.amber,
                              shape: BoxShape.circle,
                            ),
                          ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Text.rich(
                            TextSpan(
                              children: [
                                TextSpan(
                                  text: e.userEmail.isEmpty
                                      ? 'System'
                                      : e.userEmail,
                                  style: const TextStyle(
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                                TextSpan(text: ' ${describeAction(e.action)} '),
                                if (e.target.isNotEmpty)
                                  TextSpan(
                                    text: e.target,
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                    ),
                                  ),
                              ],
                            ),
                            style: const TextStyle(fontSize: 13.5, height: 1.4),
                          ),
                        ),
                        const SizedBox(width: 8),
                        Text(
                          timeAgo(e.createdAt),
                          style: TextStyle(color: c.muted, fontSize: 12),
                        ),
                      ],
                    ),
                  ),
              ],
            ),
    );
  }
}
