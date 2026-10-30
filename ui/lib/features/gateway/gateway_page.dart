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
        ref.invalidate(gatewayCertificatesProvider);
      },
      children: [
        NTabs(index: _tab, labels: const ['Routes', 'Certificates', 'Blocked IPs'], onChanged: (i) => setState(() => _tab = i)),
        const SizedBox(height: NectarTokens.gap),
        switch (_tab) {
          0 => const _Routes(),
          1 => Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [_Wildcards(canManage: session.can('settings.manage')), const SizedBox(height: NectarTokens.gap), const _Certificates()],
            ),
          _ => Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _Blocked(canManage: session.can('server.manage')),
                if (session.can('settings.manage')) ...[const SizedBox(height: NectarTokens.gap), const _RateLimitBlocking()],
              ],
            ),
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
                    r.certNotAfter == null && r.coveredUntil == null ? Icons.lock_open_rounded : Icons.verified_user_outlined,
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
    if (r.certNotAfter == null && r.coveredUntil != null) {
      final days = r.coveredUntil!.difference(DateTime.now()).inDays;
      return 'served by ${r.coveredBy} · valid until ${r.coveredUntil!.toIso8601String().substring(0, 10)} ($days days)${r.forceSsl ? ' · HTTPS forced' : ''}';
    }
    if (r.certNotAfter == null) return r.acme ? 'waiting for the first certificate (is DNS pointing here?)' : 'HTTP only';
    final days = r.certNotAfter!.difference(DateTime.now()).inDays;
    final when = r.certNotAfter!.toIso8601String().substring(0, 10);
    return '${source.isEmpty ? '' : '$source · '}valid until $when (${days < 0 ? 'expired' : '$days days'})${r.forceSsl ? ' · HTTPS forced' : ''}';
  }

  static Color _certColor(BuildContext context, GatewayRoute r) {
    final c = NectarColors.of(context);
    final until = r.certNotAfter ?? r.coveredUntil;
    if (until == null) return r.acme ? c.warning : c.muted;
    final days = until.difference(DateTime.now()).inDays;
    return days < 0 ? c.failed : (days < 14 ? c.warning : c.running);
  }
}

/// Wildcard certificates (DNS-01): the DNS provider, `*.<base domain>` in one click, and the list.
class _Wildcards extends ConsumerWidget {
  const _Wildcards({required this.canManage});

  final bool canManage;

  Future<void> _run(BuildContext context, WidgetRef ref, Future<void> Function() f, [String? done]) async {
    try {
      await f();
      ref.invalidate(gatewayCertificatesProvider);
      ref.invalidate(gatewayRoutesProvider);
      if (done != null && context.mounted) showSnack(context, done);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  static String statusPill(String s) => switch (s) { 'valid' => 'running', 'expiring' => 'warning', 'expired' || 'failed' => 'failed', _ => 'pending' };

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(gatewayCertificatesProvider);
    final c = NectarColors.of(context);
    final api = ref.read(apiProvider);
    return NCard(
      title: 'Wildcard certificates',
      child: AsyncBlock(
        value: info,
        onRetry: () => ref.invalidate(gatewayCertificatesProvider),
        data: (i) {
          if (!i.supported) {
            return Text('One certificate for *.your-domain serves every app at once, with no Let\'s Encrypt order per app. '
                'This needs nectar-gateway 1.2.2 or newer: update it under Settings → Updates.', style: TextStyle(color: c.muted));
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text('A wildcard certificate (*.example.com) is proved over DNS: the gateway adds a TXT record through your DNS '
                  'provider. Apps under it get HTTPS the moment they are created.', style: TextStyle(color: c.muted, fontSize: 13)),
              const SizedBox(height: 10),
              if (!i.acme)
                Text('The gateway\'s Let\'s Encrypt client is off: set the Let\'s Encrypt email (Settings → Gateway) first.', style: TextStyle(color: c.warning))
              else
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    Icon(i.dns ? Icons.dns_rounded : Icons.dns_outlined, size: 18, color: i.dns ? c.running : c.muted),
                    Text(i.dns ? 'DNS-01 through ${i.dnsProvider.isEmpty ? 'your provider' : i.dnsProvider}' : 'No DNS provider yet'),
                    if (canManage)
                      TextButton(
                        onPressed: () => showDialog<void>(context: context, builder: (_) => _DnsDialog(configured: i.dns)),
                        child: Text(i.dns ? 'Change token' : 'Set up Cloudflare'),
                      ),
                    if (canManage && i.dns)
                      TextButton(
                        onPressed: () async {
                          if (!await confirmAction(context, 'Remove the DNS provider?', 'Wildcard certificates stop renewing until you set it again.', 'Remove', danger: true)) return;
                          if (!context.mounted) return;
                          await _run(context, ref, () => api.put('/gateway/settings', data: {'dns_provider': ''}));
                        },
                        child: const Text('Remove'),
                      ),
                    if (canManage && i.dns && i.baseWildcard.isNotEmpty && !i.hasBaseWildcard)
                      FilledButton.icon(
                        onPressed: () => _run(context, ref, () => api.post('/gateway/certificates', data: {'base_domain': true}),
                            'Ordering ${i.baseWildcard} — this takes a minute or two'),
                        icon: const Icon(Icons.add_moderator_outlined, size: 18),
                        label: Text('Get ${i.baseWildcard}'),
                      ),
                    if (canManage && i.dns)
                      OutlinedButton(onPressed: () => showDialog<void>(context: context, builder: (_) => const _WildcardDialog()), child: const Text('Other domain…')),
                  ],
                ),
              if (i.certificates.isNotEmpty) const SizedBox(height: 8),
              for (final cert in i.certificates)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: StatusPill(status: statusPill(cert.status), label: cert.status.isEmpty ? 'Pending' : cert.status[0].toUpperCase() + cert.status.substring(1)),
                  title: Text(cert.names.join(' + '), style: monoStyle(context, size: 13.5)),
                  subtitle: Text(
                    [
                      if (cert.notAfter != null) 'valid until ${cert.notAfter!.toIso8601String().substring(0, 10)}',
                      if (cert.status == 'pending' && cert.lastError.isEmpty) 'ordering…',
                      if (cert.serves.isNotEmpty) 'serves ${cert.serves.length} host${cert.serves.length == 1 ? '' : 's'}',
                      if (cert.lastError.isNotEmpty) cert.lastError,
                    ].join(' · '),
                    style: TextStyle(fontSize: 12.5, color: cert.lastError.isNotEmpty ? c.failed : c.muted),
                  ),
                  trailing: canManage
                      ? PopupMenuButton<String>(
                          onSelected: (a) async {
                            if (a == 'renew') {
                              await _run(context, ref, () => api.post('/gateway/certificates/${cert.id}/renew'), 'Renewing ${cert.domain}');
                            } else if (await confirmAction(context, 'Delete ${cert.domain}?',
                                'Hosts it serves fall back to their own Let\'s Encrypt certificates (ordered per host).', 'Delete', danger: true)) {
                              if (context.mounted) await _run(context, ref, () => api.delete('/gateway/certificates/${cert.id}'));
                            }
                          },
                          itemBuilder: (_) => [
                            if (cert.acme) const PopupMenuItem(value: 'renew', child: Text('Renew now')),
                            const PopupMenuItem(value: 'delete', child: Text('Delete')),
                          ],
                        )
                      : null,
                ),
            ],
          );
        },
      ),
    );
  }
}

class _DnsDialog extends ConsumerStatefulWidget {
  const _DnsDialog({required this.configured});

  final bool configured;

  @override
  ConsumerState<_DnsDialog> createState() => _DnsDialogState();
}

class _DnsDialogState extends ConsumerState<_DnsDialog> {
  final _token = TextEditingController();
  bool _busy = false;

  @override
  void dispose() {
    _token.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    if (_token.text.trim().isEmpty) return;
    setState(() => _busy = true);
    try {
      await ref.read(apiProvider).put('/gateway/settings', data: {'dns_provider': 'cloudflare', 'dns_api_token': _token.text.trim()});
      ref.invalidate(gatewayCertificatesProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(widget.configured ? 'Change the Cloudflare token' : 'Cloudflare for wildcard certificates'),
      content: SizedBox(
        width: 460,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('In Cloudflare: My Profile → API Tokens → Create Token → "Edit zone DNS", limited to your zone '
                '(permissions Zone:Read and DNS:Edit). The token is stored in the gateway and never shown again.',
                style: TextStyle(color: c.muted, fontSize: 13)),
            const SizedBox(height: 12),
            TextField(controller: _token, obscureText: true, autofocus: true, decoration: const InputDecoration(labelText: 'API token'), onSubmitted: (_) => _save()),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _save, child: const Text('Save')),
      ],
    );
  }
}

class _WildcardDialog extends ConsumerStatefulWidget {
  const _WildcardDialog();

  @override
  ConsumerState<_WildcardDialog> createState() => _WildcardDialogState();
}

class _WildcardDialogState extends ConsumerState<_WildcardDialog> {
  final _domain = TextEditingController(text: '*.');
  bool _apex = true;
  bool _busy = false;

  @override
  void dispose() {
    _domain.dispose();
    super.dispose();
  }

  Future<void> _create() async {
    setState(() => _busy = true);
    try {
      await ref.read(apiProvider).post('/gateway/certificates', data: {'domain': _domain.text.trim(), 'include_apex': _apex});
      ref.invalidate(gatewayCertificatesProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
        title: const Text('Wildcard certificate'),
        content: SizedBox(
          width: 420,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: _domain, autofocus: true, decoration: const InputDecoration(labelText: 'Domain', hintText: '*.example.com')),
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                value: _apex,
                onChanged: (v) => setState(() => _apex = v ?? true),
                title: const Text('Also the domain itself (example.com)'),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
          FilledButton(onPressed: _busy ? null : _create, child: const Text('Order')),
        ],
      );
}

/// How long a client that keeps exceeding a domain's rate limit is blocked (gateway setting).
class _RateLimitBlocking extends ConsumerWidget {
  const _RateLimitBlocking();

  static const _choices = {0: 'Never (only 429)', 10: '10 minutes', 60: '1 hour', 1440: '24 hours'};

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(gatewaySettingsProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: 'Rate limits',
      child: AsyncBlock(
        value: settings,
        onRetry: () => ref.invalidate(gatewaySettingsProvider),
        data: (m) {
          if (!m.containsKey('rate_limit_block_minutes')) {
            return Text('This nectar-gateway blocks a client over a rate limit for 24 hours. Update it to answer 429 instead.',
                style: TextStyle(color: c.muted));
          }
          final cur = (m['rate_limit_block_minutes'] as num?)?.toInt() ?? 10;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text('Above a domain\'s rate limit a client gets 429 and may retry a second later. A client that keeps going '
                  '(more than ten times the limit refused within a minute) is blocked:', style: TextStyle(color: c.muted, fontSize: 13)),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                children: [
                  for (final e in {..._choices, if (!_choices.containsKey(cur)) cur: '$cur minutes'}.entries)
                    ChoiceChip(
                      label: Text(e.value),
                      selected: e.key == cur,
                      onSelected: (_) async {
                        try {
                          await ref.read(apiProvider).put('/gateway/settings', data: {'rate_limit_block_minutes': e.key});
                          ref.invalidate(gatewaySettingsProvider);
                        } on Object catch (err) {
                          if (context.mounted) showSnack(context, describeError(err), error: true);
                        }
                      },
                    ),
                ],
              ),
            ],
          );
        },
      ),
    );
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
