import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';

/// Gateway: every route nectar-gateway serves with its health, TLS certificate and rate limit, and the
/// blocked client IPs (the gateway's scanner blocks automatically; you can block and unblock by hand).
class GatewayPage extends ConsumerStatefulWidget {
  const GatewayPage({super.key});

  @override
  ConsumerState<GatewayPage> createState() => _GatewayPageState();
}

class _GatewayPageState extends ConsumerState<GatewayPage> {
  int _tab = 0;

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(sessionProvider);
    if (!session.can('server.view')) {
      return const PageScaffold(
        title: 'Gateway',
        children: [NCard(child: EmptyState(icon: Icons.lock_outline_rounded, title: 'No access', message: 'Your role does not include server.view.'))],
      );
    }
    return PageScaffold(
      title: 'Gateway',
      subtitle: 'nectar-gateway serves every domain on ports 80/443: routes, certificates, rate limits and blocked IPs.',
      onRefresh: () async {
        ref.invalidate(gatewayRoutesProvider);
        ref.invalidate(gatewayHealthProvider);
        ref.invalidate(blockedIpsProvider);
      },
      children: [
        NTabs(index: _tab, labels: const ['Routes', 'Certificates', 'Blocked IPs'], onChanged: (i) => setState(() => _tab = i)),
        const SizedBox(height: NectarTokens.gap),
        switch (_tab) {
          0 => const _Routes(),
          1 => const _Certificates(),
          _ => _Blocked(canManage: session.can('server.manage')),
        },
      ],
    );
  }
}

String _healthTone(RouteHealth? h) => h == null || h.up == null ? 'stopped' : (h.up! ? 'running' : 'failed');

class _Routes extends ConsumerWidget {
  const _Routes();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final routes = ref.watch(gatewayRoutesProvider);
    final health = ref.watch(gatewayHealthProvider).value ?? const {};
    final c = NectarColors.of(context);
    return NCard(
      child: AsyncBlock(
        value: routes,
        onRetry: () => ref.invalidate(gatewayRoutesProvider),
        data: (list) {
          if (list.isEmpty) return const EmptyState(icon: Icons.alt_route_rounded, title: 'No routes', message: 'Domains of your apps appear here.');
          final down = list.where((r) => health[r.id]?.up == false).length;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                '${list.length} routes${down > 0 ? ' · $down failing their health check' : ''}',
                style: TextStyle(color: down > 0 ? c.failed : c.muted, fontSize: 12.5),
              ),
              const SizedBox(height: 6),
              for (final r in list) ...[
                const Divider(height: 1),
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 9),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Padding(padding: const EdgeInsets.only(top: 4), child: StatusDot(status: r.hcEnabled ? _healthTone(health[r.id]) : 'stopped')),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(r.label, style: monoStyle(context, size: 13.5)),
                            Text(
                              [
                                r.mode == 'redirect' ? 'redirect → ${r.target}' : r.target,
                                if (r.maintenance) 'maintenance page',
                                if (r.rateLimit > 0) '${r.rateLimit} req/s per IP' else 'no rate limit',
                                if (r.managedBy == 'nectar-apps') 'managed by the panel' else if (r.managedBy.isNotEmpty) 'managed by ${r.managedBy}',
                                if (!r.hcEnabled) 'no health check',
                                if (health[r.id]?.lastError.isNotEmpty ?? false) health[r.id]!.lastError,
                              ].join(' · '),
                              maxLines: 3,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(fontSize: 12, color: health[r.id]?.up == false ? c.failed : c.muted),
                            ),
                          ],
                        ),
                      ),
                      Icon(r.certNotAfter != null ? Icons.lock_rounded : Icons.lock_open_rounded, size: 16, color: r.certNotAfter != null ? c.running : c.muted),
                    ],
                  ),
                ),
              ],
              const SizedBox(height: 8),
              Text('Rate limits, timeouts and body limits are set per domain on the app page (Domains → edit).',
                  style: TextStyle(color: c.muted, fontSize: 12.5)),
            ],
          );
        },
      ),
    );
  }
}

class _Certificates extends ConsumerWidget {
  const _Certificates();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final routes = ref.watch(gatewayRoutesProvider);
    final c = NectarColors.of(context);
    return NCard(
      child: AsyncBlock(
        value: routes,
        onRetry: () => ref.invalidate(gatewayRoutesProvider),
        data: (all) {
          final list = all.where((r) => r.mode != 'redirect' || r.certNotAfter != null).toList()
            ..sort((a, b) => (a.certNotAfter ?? DateTime(9999)).compareTo(b.certNotAfter ?? DateTime(9999)));
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final r in list)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: Icon(
                    r.certNotAfter == null ? Icons.lock_open_rounded : Icons.verified_user_outlined,
                    color: _certColor(context, r),
                  ),
                  title: Text(r.label, style: monoStyle(context, size: 13.5)),
                  subtitle: Text(_certText(r), style: TextStyle(fontSize: 12.5, color: _certColor(context, r))),
                ),
              const SizedBox(height: 6),
              Text('Let\'s Encrypt certificates renew automatically about 30 days before they expire; the "Certificate expiring" alert '
                  'warns when one does not.', style: TextStyle(color: c.muted, fontSize: 12.5)),
            ],
          );
        },
      ),
    );
  }

  static String _certText(GatewayRoute r) {
    final source = switch (r.certSource) { 'acme' => 'Let\'s Encrypt', 'upload' => 'uploaded', 'file' => 'file', _ => '' };
    if (r.certNotAfter == null) return r.acme ? 'waiting for the first certificate (is DNS pointing here?)' : 'HTTP only';
    final days = r.certNotAfter!.difference(DateTime.now()).inDays;
    final when = r.certNotAfter!.toIso8601String().substring(0, 10);
    return '${source.isEmpty ? '' : '$source · '}valid until $when (${days < 0 ? 'expired' : '$days days'})${r.forceSsl ? ' · HTTPS forced' : ''}';
  }

  static Color _certColor(BuildContext context, GatewayRoute r) {
    final c = NectarColors.of(context);
    if (r.certNotAfter == null) return r.acme ? c.warning : c.muted;
    final days = r.certNotAfter!.difference(DateTime.now()).inDays;
    return days < 0 ? c.failed : (days < 14 ? c.warning : c.running);
  }
}

class _Blocked extends ConsumerWidget {
  const _Blocked({required this.canManage});

  final bool canManage;

  Future<void> _unblock(BuildContext context, WidgetRef ref, BlockedIp b) async {
    try {
      await ref.read(apiProvider).delete('/gateway/blocked/${Uri.encodeComponent(b.ip)}');
      ref.invalidate(blockedIpsProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final blocked = ref.watch(blockedIpsProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: 'Blocked IPs',
      trailing: canManage
          ? FilledButton.icon(
              onPressed: () => showDialog<void>(context: context, builder: (_) => const _BlockDialog()),
              icon: const Icon(Icons.block_rounded, size: 18),
              label: const Text('Block IP'),
            )
          : null,
      child: AsyncBlock(
        value: blocked,
        onRetry: () => ref.invalidate(blockedIpsProvider),
        data: (list) => list.isEmpty
            ? Text('Nobody is blocked. The gateway blocks scanners (requests for .env, wp-login …) by itself.', style: TextStyle(color: c.muted))
            : Column(
                children: [
                  for (final b in list)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      leading: Icon(Icons.block_rounded, color: c.failed),
                      title: Text(b.ip, style: monoStyle(context, size: 13.5)),
                      subtitle: Text(
                        [
                          b.reason,
                          if (b.blockedAt != null) 'since ${timeAgo(b.blockedAt)}',
                          if (b.expiresAt != null && b.expiresAt!.year < DateTime.now().year + 5) 'until ${b.expiresAt!.toLocal().toString().substring(0, 16)}',
                        ].join(' · '),
                        style: TextStyle(fontSize: 12.5, color: c.muted),
                      ),
                      trailing: canManage ? TextButton(onPressed: () => _unblock(context, ref, b), child: const Text('Unblock')) : null,
                    ),
                ],
              ),
      ),
    );
  }
}

class _BlockDialog extends ConsumerStatefulWidget {
  const _BlockDialog();

  @override
  ConsumerState<_BlockDialog> createState() => _BlockDialogState();
}

class _BlockDialogState extends ConsumerState<_BlockDialog> {
  final _ip = TextEditingController();
  final _reason = TextEditingController();
  int _hours = 24;

  @override
  void dispose() {
    _ip.dispose();
    _reason.dispose();
    super.dispose();
  }

  Future<void> _block() async {
    try {
      await ref.read(apiProvider).post('/gateway/blocked', data: {'ip': _ip.text.trim(), 'reason': _reason.text.trim(), 'ttl_seconds': _hours * 3600});
      ref.invalidate(blockedIpsProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
        title: const Text('Block an IP'),
        content: SizedBox(
          width: 420,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: _ip, decoration: const InputDecoration(labelText: 'IP address', hintText: '203.0.113.9')),
              const SizedBox(height: 10),
              TextField(controller: _reason, decoration: const InputDecoration(labelText: 'Reason (optional)')),
              const SizedBox(height: 10),
              DropdownButtonFormField<int>(
                initialValue: _hours,
                decoration: const InputDecoration(labelText: 'For'),
                items: const [
                  DropdownMenuItem(value: 1, child: Text('1 hour')),
                  DropdownMenuItem(value: 24, child: Text('1 day')),
                  DropdownMenuItem(value: 168, child: Text('1 week')),
                  DropdownMenuItem(value: 0, child: Text('until unblocked')),
                ],
                onChanged: (v) => setState(() => _hours = v ?? 24),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
          FilledButton(onPressed: _block, child: const Text('Block')),
        ],
      );
}
