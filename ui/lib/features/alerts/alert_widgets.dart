import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/models.dart';
import '../../core/format.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// Titles of the built-in rules (the catalog from the API has the same; lists don't need to load it).
const alertRuleTitles = {
  'app_down': 'App is down',
  'addon_down': 'Database is down',
  'app_crashloop': 'Crash loop',
  'app_oom': 'Out of memory',
  'node_not_ready': 'Node not ready',
  'node_cpu': 'High CPU',
  'node_memory': 'High memory',
  'node_disk': 'Disk almost full',
  'cert_expiry': 'Certificate expiring',
  'gateway_down': 'Gateway unreachable',
  'backup_failed': 'Backup failed',
  'update_available': 'Update available',
  'update_failed': 'Update failed',
  'deploy_failed': 'Deploy failed',
  'deploy_succeeded': 'Deploy finished',
};

String alertTitle(AlertIncident a) => alertRuleTitles[a.rule] ?? a.rule;

/// Where an alert leads in the panel.
String? alertTarget(AlertIncident a) {
  if (a.appId != null) return '/apps/${a.appId}';
  if (a.rule == 'addon_down' && a.key.contains(':')) return '/addons/${a.key.split(':').last}';
  if (a.rule.startsWith('node_')) return '/server';
  if (a.rule == 'backup_failed') return '/backups';
  if (a.rule.startsWith('update_')) return '/updates';
  return null;
}

String _span(Duration d) {
  if (d.inMinutes < 1) return '${d.inSeconds}s';
  if (d.inHours < 1) return '${d.inMinutes}m';
  if (d.inDays < 2) return '${d.inHours}h ${d.inMinutes % 60}m';
  return '${d.inDays} days';
}

/// One incident row: severity icon, rule title, subject, message and timing.
class AlertTile extends StatelessWidget {
  const AlertTile({super.key, required this.alert, this.dense = false});

  final AlertIncident alert;
  final bool dense;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final a = alert;
    final color = !a.firing ? c.running : (a.critical ? c.failed : c.warning);
    final icon = !a.firing ? Icons.check_circle_rounded : (a.critical ? Icons.error_rounded : Icons.warning_amber_rounded);
    final started = a.startedAt;
    final timing = a.firing
        ? 'since ${timeAgo(started)}'
        : (a.resolvedAt != null && started != null ? 'lasted ${_span(a.resolvedAt!.difference(started))} · ${timeAgo(a.resolvedAt)}' : 'resolved');
    final target = alertTarget(a);
    final body = Padding(
      padding: EdgeInsets.symmetric(vertical: dense ? 6 : 10, horizontal: 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(icon, color: color, size: 20),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Wrap(
                  spacing: 8,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    Text(alertTitle(a), style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14)),
                    Text(a.subject, style: TextStyle(color: c.muted, fontSize: 13)),
                  ],
                ),
                const SizedBox(height: 2),
                Text(a.message, style: const TextStyle(fontSize: 13.5)),
                const SizedBox(height: 2),
                Text(
                  [timing, if (a.firing && a.notified == 0) 'no channel notified'].join(' · '),
                  style: TextStyle(color: c.muted, fontSize: 12),
                ),
              ],
            ),
          ),
          if (target != null) Icon(Icons.chevron_right_rounded, color: c.muted),
        ],
      ),
    );
    if (target == null) return body;
    return InkWell(borderRadius: BorderRadius.circular(NectarTokens.radiusSmall), onTap: () => context.go(target), child: body);
  }
}

/// Compact card listing firing alerts (dashboard, app page). Renders nothing when there are none.
class FiringAlertsCard extends StatelessWidget {
  const FiringAlertsCard({super.key, required this.alerts, this.showAll = true});

  final List<AlertIncident> alerts;
  final bool showAll;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    if (alerts.isEmpty) return const SizedBox.shrink();
    final c = NectarColors.of(context);
    final critical = alerts.any((a) => a.critical);
    return Padding(
      padding: const EdgeInsets.only(bottom: NectarTokens.gap),
      child: Container(
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(NectarTokens.radius),
          border: Border.all(color: (critical ? c.failed : c.warning).withValues(alpha: 0.6)),
          color: (critical ? c.failed : c.warning).withValues(alpha: 0.06),
        ),
        padding: const EdgeInsets.fromLTRB(14, 12, 14, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    alerts.length == 1 ? '1 alert firing' : '${alerts.length} alerts firing',
                    style: Theme.of(context).textTheme.titleSmall,
                  ),
                ),
                if (showAll) TextButton(onPressed: () => context.go('/alerts'), child: Text(s.alertAllAlerts)),
              ],
            ),
            for (final a in alerts.take(5)) AlertTile(alert: a, dense: true),
            if (alerts.length > 5)
              Padding(
                padding: const EdgeInsets.only(bottom: 6),
                child: Text(s.alertAndMore(alerts.length - 5), style: TextStyle(color: c.muted, fontSize: 12.5)),
              ),
          ],
        ),
      ),
    );
  }
}

/// Firing alerts of one app (app page overview).
class AppAlertsBanner extends StatelessWidget {
  const AppAlertsBanner({super.key, required this.value});

  final AsyncValue<List<AlertIncident>> value;

  @override
  Widget build(BuildContext context) {
    final list = value.hasValue ? value.value! : const <AlertIncident>[];
    return FiringAlertsCard(alerts: list, showAll: false);
  }
}

/// Formats a rule duration (seconds) for the rules editor.
String formatFor(int seconds) {
  if (seconds == 0) return 'immediately';
  if (seconds < 60) return '${seconds}s';
  if (seconds < 3600) return '${seconds ~/ 60} min';
  return seconds % 3600 == 0 ? '${seconds ~/ 3600} h' : '${seconds ~/ 60} min';
}

/// Empty state of alert lists.
Widget noAlerts(String message) => EmptyState(icon: Icons.notifications_off_outlined, title: 'All quiet', message: message);
