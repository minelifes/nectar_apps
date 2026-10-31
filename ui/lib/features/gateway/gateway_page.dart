import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';

/// Gateway: every route nectar-gateway serves with its health, TLS certificate and rate limit, and the
/// latest requests over all domains, the blocked client IPs (the gateway's scanner blocks automatically; you can block and unblock by hand).
class GatewayPage extends ConsumerStatefulWidget {
  const GatewayPage({super.key});

  @override
  ConsumerState<GatewayPage> createState() => _GatewayPageState();
}

class _GatewayPageState extends ConsumerState<GatewayPage> {
  int _tab = 0;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = ref.watch(sessionProvider);
    if (!session.can('server.view')) {
      return PageScaffold(
        title: s.serverGateway,
        children: [
          NCard(
            child: EmptyState(
              icon: Icons.lock_outline_rounded,
              title: s.noAccess,
              message: s.gatewayYourRoleDoesNotInclude,
            ),
          ),
        ],
      );
    }
    return PageScaffold(
      title: s.serverGateway,
      subtitle: s.gatewayNectarGatewayServesEveryDomain,
      onRefresh: () async {
        ref.invalidate(gatewayRoutesProvider);
        ref.invalidate(gatewayHealthProvider);
        ref.invalidate(blockedIpsProvider);
        ref.invalidate(gatewayCertificatesProvider);
        ref.invalidate(gatewayAccessLogProvider);
      },
      children: [
        NTabs(
          index: _tab,
          labels: const ['Routes', 'Access log', 'Certificates', 'Blocked IPs'],
          onChanged: (i) => setState(() => _tab = i),
        ),
        const SizedBox(height: NectarTokens.gap),
        switch (_tab) {
          0 => const _Routes(),
          1 => _AccessLogView(canBlock: session.can('server.manage')),
          2 => Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _Wildcards(canManage: session.can('settings.manage')),
              const SizedBox(height: NectarTokens.gap),
              const _Certificates(),
            ],
          ),
          _ => Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _Blocked(canManage: session.can('server.manage')),
              if (session.can('settings.manage')) ...[
                const SizedBox(height: NectarTokens.gap),
                const _RateLimitBlocking(),
              ],
            ],
          ),
        },
      ],
    );
  }
}

String _healthTone(RouteHealth? h) =>
    h == null || h.up == null ? 'stopped' : (h.up! ? 'running' : 'failed');

class _Routes extends ConsumerWidget {
  const _Routes();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final routes = ref.watch(gatewayRoutesProvider);
    final health = ref.watch(gatewayHealthProvider).value ?? const {};
    final c = NectarColors.of(context);
    return NCard(
      child: AsyncBlock(
        value: routes,
        onRetry: () => ref.invalidate(gatewayRoutesProvider),
        data: (list) {
          if (list.isEmpty) {
            return EmptyState(
              icon: Icons.alt_route_rounded,
              title: l.gatewayNoRoutes,
              message: l.gatewayDomainsOfYourAppsAppear,
            );
          }
          final down = list.where((r) => health[r.id]?.up == false).length;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                '${list.length} routes${down > 0 ? ' · $down failing their health check' : ''}',
                style: TextStyle(
                  color: down > 0 ? c.failed : c.muted,
                  fontSize: 12.5,
                ),
              ),
              const SizedBox(height: 6),
              for (final r in list) ...[
                const Divider(height: 1),
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 9),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Padding(
                        padding: const EdgeInsets.only(top: 4),
                        child: StatusDot(
                          status: r.hcEnabled
                              ? _healthTone(health[r.id])
                              : 'stopped',
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              r.label,
                              style: monoStyle(context, size: 13.5),
                            ),
                            Text(
                              [
                                r.mode == 'redirect'
                                    ? 'redirect → ${r.target}'
                                    : r.target,
                                if (r.maintenance) 'maintenance page',
                                if (r.rateLimit > 0)
                                  '${r.rateLimit} req/s per IP'
                                else
                                  'no rate limit',
                                if (r.managedBy == 'nectar-apps')
                                  'managed by the panel'
                                else if (r.managedBy.isNotEmpty)
                                  'managed by ${r.managedBy}',
                                if (!r.hcEnabled) 'no health check',
                                if (health[r.id]?.lastError.isNotEmpty ?? false)
                                  health[r.id]!.lastError,
                              ].join(' · '),
                              maxLines: 3,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontSize: 12,
                                color: health[r.id]?.up == false
                                    ? c.failed
                                    : c.muted,
                              ),
                            ),
                          ],
                        ),
                      ),
                      Icon(
                        r.certNotAfter != null
                            ? Icons.lock_rounded
                            : Icons.lock_open_rounded,
                        size: 16,
                        color: r.certNotAfter != null ? c.running : c.muted,
                      ),
                    ],
                  ),
                ),
              ],
              const SizedBox(height: 8),
              Text(
                l.gatewayRateLimitsTimeoutsAndBody,
                style: TextStyle(color: c.muted, fontSize: 12.5),
              ),
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
    final s = S.of(context);
    final routes = ref.watch(gatewayRoutesProvider);
    final c = NectarColors.of(context);
    return NCard(
      child: AsyncBlock(
        value: routes,
        onRetry: () => ref.invalidate(gatewayRoutesProvider),
        data: (all) {
          final list =
              all
                  .where((r) => r.mode != 'redirect' || r.certNotAfter != null)
                  .toList()
                ..sort(
                  (a, b) => (a.certNotAfter ?? DateTime(9999)).compareTo(
                    b.certNotAfter ?? DateTime(9999),
                  ),
                );
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final r in list)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: Icon(
                    r.certNotAfter == null && r.coveredUntil == null
                        ? Icons.lock_open_rounded
                        : Icons.verified_user_outlined,
                    color: _certColor(context, r),
                  ),
                  title: Text(r.label, style: monoStyle(context, size: 13.5)),
                  subtitle: Text(
                    _certText(r),
                    style: TextStyle(
                      fontSize: 12.5,
                      color: _certColor(context, r),
                    ),
                  ),
                ),
              const SizedBox(height: 6),
              Text(
                s.gatewayLetSEncryptCertificatesRenew,
                style: TextStyle(color: c.muted, fontSize: 12.5),
              ),
            ],
          );
        },
      ),
    );
  }

  static String _certText(GatewayRoute r) {
    final source = switch (r.certSource) {
      'acme' => 'Let\'s Encrypt',
      'upload' => 'uploaded',
      'file' => 'file',
      _ => '',
    };
    if (r.certNotAfter == null && r.coveredUntil != null) {
      final days = r.coveredUntil!.difference(DateTime.now()).inDays;
      return 'served by ${r.coveredBy} · valid until ${r.coveredUntil!.toIso8601String().substring(0, 10)} ($days days)${r.forceSsl ? ' · HTTPS forced' : ''}';
    }
    if (r.certNotAfter == null) {
      return r.acme
          ? 'waiting for the first certificate (is DNS pointing here?)'
          : 'HTTP only';
    }
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

  Future<void> _run(
    BuildContext context,
    WidgetRef ref,
    Future<void> Function() f, [
    String? done,
  ]) async {
    try {
      await f();
      ref.invalidate(gatewayCertificatesProvider);
      ref.invalidate(gatewayRoutesProvider);
      if (done != null && context.mounted) showSnack(context, done);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  static String statusPill(String s) => switch (s) {
    'valid' => 'running',
    'expiring' => 'warning',
    'expired' || 'failed' => 'failed',
    _ => 'pending',
  };

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final info = ref.watch(gatewayCertificatesProvider);
    final c = NectarColors.of(context);
    final api = ref.read(apiProvider);
    return NCard(
      title: l.gatewayWildcardCertificates,
      child: AsyncBlock(
        value: info,
        onRetry: () => ref.invalidate(gatewayCertificatesProvider),
        data: (i) {
          if (!i.supported) {
            return Text(
              l.gatewayOneCertificateForYourDomain,
              style: TextStyle(color: c.muted),
            );
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                l.gatewayAWildcardCertificateExampleCom,
                style: TextStyle(color: c.muted, fontSize: 13),
              ),
              const SizedBox(height: 10),
              if (!i.acme)
                Text(
                  l.gatewayTheGatewaySLetS,
                  style: TextStyle(color: c.warning),
                )
              else
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    Icon(
                      i.dns ? Icons.dns_rounded : Icons.dns_outlined,
                      size: 18,
                      color: i.dns ? c.running : c.muted,
                    ),
                    Text(
                      i.dns
                          ? 'DNS-01 through ${i.dnsProvider.isEmpty ? 'your provider' : i.dnsProvider}'
                          : 'No DNS provider yet',
                    ),
                    if (canManage)
                      TextButton(
                        onPressed: () => showDialog<void>(
                          context: context,
                          builder: (_) => _DnsDialog(configured: i.dns),
                        ),
                        child: Text(
                          i.dns ? 'Change token' : 'Set up Cloudflare',
                        ),
                      ),
                    if (canManage && i.dns)
                      TextButton(
                        onPressed: () async {
                          if (!await confirmAction(
                            context,
                            l.gatewayRemoveTheDnsProvider,
                            l.gatewayWildcardCertificatesStopRenewingUntil,
                            l.backupRemove2,
                            danger: true,
                          )) {
                            return;
                          }
                          if (!context.mounted) return;
                          await _run(
                            context,
                            ref,
                            () => api.put(
                              '/gateway/settings',
                              data: {'dns_provider': ''},
                            ),
                          );
                        },
                        child: Text(l.backupRemove2),
                      ),
                    if (canManage &&
                        i.dns &&
                        i.baseWildcard.isNotEmpty &&
                        !i.hasBaseWildcard)
                      FilledButton.icon(
                        onPressed: () => _run(
                          context,
                          ref,
                          () => api.post(
                            '/gateway/certificates',
                            data: {'base_domain': true},
                          ),
                          'Ordering ${i.baseWildcard} — this takes a minute or two',
                        ),
                        icon: const Icon(
                          Icons.add_moderator_outlined,
                          size: 18,
                        ),
                        label: Text(l.gatewayGet(i.baseWildcard)),
                      ),
                    if (canManage && i.dns)
                      OutlinedButton(
                        onPressed: () => showDialog<void>(
                          context: context,
                          builder: (_) => const _WildcardDialog(),
                        ),
                        child: Text(l.gatewayOtherDomain),
                      ),
                  ],
                ),
              if (i.certificates.isNotEmpty) const SizedBox(height: 8),
              for (final cert in i.certificates)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: StatusPill(
                    status: statusPill(cert.status),
                    label: cert.status.isEmpty
                        ? 'Pending'
                        : cert.status[0].toUpperCase() +
                              cert.status.substring(1),
                  ),
                  title: Text(
                    cert.names.join(' + '),
                    style: monoStyle(context, size: 13.5),
                  ),
                  subtitle: Text(
                    [
                      if (cert.notAfter != null)
                        'valid until ${cert.notAfter!.toIso8601String().substring(0, 10)}',
                      if (cert.status == 'pending' && cert.lastError.isEmpty)
                        'ordering…',
                      if (cert.serves.isNotEmpty)
                        'serves ${cert.serves.length} host${cert.serves.length == 1 ? '' : 's'}',
                      if (cert.lastError.isNotEmpty) cert.lastError,
                    ].join(' · '),
                    style: TextStyle(
                      fontSize: 12.5,
                      color: cert.lastError.isNotEmpty ? c.failed : c.muted,
                    ),
                  ),
                  trailing: canManage
                      ? PopupMenuButton<String>(
                          onSelected: (a) async {
                            if (a == 'renew') {
                              await _run(
                                context,
                                ref,
                                () => api.post(
                                  '/gateway/certificates/${cert.id}/renew',
                                ),
                                'Renewing ${cert.domain}',
                              );
                            } else if (await confirmAction(
                              context,
                              l.gatewayDelete(cert.domain),
                              l.gatewayHostsItServesFallBack,
                              l.actionDelete,
                              danger: true,
                            )) {
                              if (context.mounted) {
                                await _run(
                                  context,
                                  ref,
                                  () => api.delete(
                                    '/gateway/certificates/${cert.id}',
                                  ),
                                );
                              }
                            }
                          },
                          itemBuilder: (_) => [
                            if (cert.acme)
                              PopupMenuItem(
                                value: 'renew',
                                child: Text(l.gatewayRenewNow),
                              ),
                            PopupMenuItem(
                              value: 'delete',
                              child: Text(l.actionDelete),
                            ),
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
      await ref
          .read(apiProvider)
          .put(
            '/gateway/settings',
            data: {
              'dns_provider': 'cloudflare',
              'dns_api_token': _token.text.trim(),
            },
          );
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
    final s = S.of(context);
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(
        widget.configured
            ? 'Change the Cloudflare token'
            : 'Cloudflare for wildcard certificates',
      ),
      content: SizedBox(
        width: 460,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              s.gatewayInCloudflareMyProfileApi,
              style: TextStyle(color: c.muted, fontSize: 13),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _token,
              obscureText: true,
              autofocus: true,
              decoration: InputDecoration(labelText: s.gatewayApiToken),
              onSubmitted: (_) => _save(),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: _busy ? null : _save,
          child: Text(s.actionSave),
        ),
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
      await ref
          .read(apiProvider)
          .post(
            '/gateway/certificates',
            data: {'domain': _domain.text.trim(), 'include_apex': _apex},
          );
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
    final s = S.of(context);
    return AlertDialog(
      title: Text(s.gatewayWildcardCertificate),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: _domain,
              autofocus: true,
              decoration: InputDecoration(
                labelText: s.gatewayDomain,
                hintText: s.gatewayExampleCom,
              ),
            ),
            CheckboxListTile(
              contentPadding: EdgeInsets.zero,
              value: _apex,
              onChanged: (v) => setState(() => _apex = v ?? true),
              title: Text(s.gatewayAlsoTheDomainItselfExample),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: _busy ? null : _create,
          child: Text(s.gatewayOrder),
        ),
      ],
    );
  }
}

/// How long a client that keeps exceeding a domain's rate limit is blocked (gateway setting).
class _RateLimitBlocking extends ConsumerWidget {
  const _RateLimitBlocking();

  static const _choices = {
    0: 'Never (only 429)',
    10: '10 minutes',
    60: '1 hour',
    1440: '24 hours',
  };

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final settings = ref.watch(gatewaySettingsProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: s.gatewayRateLimits,
      child: AsyncBlock(
        value: settings,
        onRetry: () => ref.invalidate(gatewaySettingsProvider),
        data: (m) {
          if (!m.containsKey('rate_limit_block_minutes')) {
            return Text(
              s.gatewayThisNectarGatewayBlocksA,
              style: TextStyle(color: c.muted),
            );
          }
          final cur = (m['rate_limit_block_minutes'] as num?)?.toInt() ?? 10;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                s.gatewayAboveADomainSRate,
                style: TextStyle(color: c.muted, fontSize: 13),
              ),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                children: [
                  for (final e in {
                    ..._choices,
                    if (!_choices.containsKey(cur)) cur: '$cur minutes',
                  }.entries)
                    ChoiceChip(
                      label: Text(e.value),
                      selected: e.key == cur,
                      onSelected: (_) async {
                        try {
                          await ref
                              .read(apiProvider)
                              .put(
                                '/gateway/settings',
                                data: {'rate_limit_block_minutes': e.key},
                              );
                          ref.invalidate(gatewaySettingsProvider);
                        } on Object catch (err) {
                          if (context.mounted) {
                            showSnack(context, describeError(err), error: true);
                          }
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

  Future<void> _unblock(
    BuildContext context,
    WidgetRef ref,
    BlockedIp b,
  ) async {
    try {
      await ref
          .read(apiProvider)
          .delete('/gateway/blocked/${Uri.encodeComponent(b.ip)}');
      ref.invalidate(blockedIpsProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final blocked = ref.watch(blockedIpsProvider);
    final c = NectarColors.of(context);
    return NCard(
      title: s.gatewayBlockedIps,
      trailing: canManage
          ? FilledButton.icon(
              onPressed: () => showDialog<void>(
                context: context,
                builder: (_) => const _BlockDialog(),
              ),
              icon: const Icon(Icons.block_rounded, size: 18),
              label: Text(s.gatewayBlockIp),
            )
          : null,
      child: AsyncBlock(
        value: blocked,
        onRetry: () => ref.invalidate(blockedIpsProvider),
        data: (list) => list.isEmpty
            ? Text(
                s.gatewayNobodyIsBlockedTheGateway,
                style: TextStyle(color: c.muted),
              )
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
                          if (b.blockedAt != null)
                            'since ${timeAgo(b.blockedAt)}',
                          if (b.expiresAt != null &&
                              b.expiresAt!.year < DateTime.now().year + 5)
                            'until ${b.expiresAt!.toLocal().toString().substring(0, 16)}',
                        ].join(' · '),
                        style: TextStyle(fontSize: 12.5, color: c.muted),
                      ),
                      trailing: canManage
                          ? TextButton(
                              onPressed: () => _unblock(context, ref, b),
                              child: Text(s.gatewayUnblock),
                            )
                          : null,
                    ),
                ],
              ),
      ),
    );
  }
}

class _BlockDialog extends ConsumerStatefulWidget {
  const _BlockDialog({this.ip = ''});

  final String ip;

  @override
  ConsumerState<_BlockDialog> createState() => _BlockDialogState();
}

class _BlockDialogState extends ConsumerState<_BlockDialog> {
  late final _ip = TextEditingController(text: widget.ip);
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
      await ref
          .read(apiProvider)
          .post(
            '/gateway/blocked',
            data: {
              'ip': _ip.text.trim(),
              'reason': _reason.text.trim(),
              'ttl_seconds': _hours * 3600,
            },
          );
      ref.invalidate(blockedIpsProvider);
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return AlertDialog(
      title: Text(s.gatewayBlockAnIp),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: _ip,
              decoration: InputDecoration(
                labelText: s.gatewayIpAddress,
                hintText: '203.0.113.9',
              ),
            ),
            const SizedBox(height: 10),
            TextField(
              controller: _reason,
              decoration: InputDecoration(labelText: s.gatewayReasonOptional),
            ),
            const SizedBox(height: 10),
            DropdownButtonFormField<int>(
              initialValue: _hours,
              decoration: InputDecoration(labelText: s.gatewayFor),
              items: [
                DropdownMenuItem(value: 1, child: Text(s.gateway1Hour)),
                DropdownMenuItem(value: 24, child: Text(s.gateway1Day)),
                DropdownMenuItem(value: 168, child: Text(s.gateway1Week)),
                DropdownMenuItem(
                  value: 0,
                  child: Text(s.gatewayUntilUnblocked),
                ),
              ],
              onChanged: (v) => setState(() => _hours = v ?? 24),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(onPressed: _block, child: Text(s.gatewayBlock)),
      ],
    );
  }
}

/// The gateway's latest requests over all domains (kept in the gateway's memory, newest first), with
/// filters; an IP can be blocked from its row.
class _AccessLogView extends ConsumerStatefulWidget {
  const _AccessLogView({required this.canBlock});

  final bool canBlock;

  @override
  ConsumerState<_AccessLogView> createState() => _AccessLogViewState();
}

class _AccessLogViewState extends ConsumerState<_AccessLogView> {
  String _domain = '';
  String _status = '';
  String _ip = '';
  final _ipField = TextEditingController();

  @override
  void dispose() {
    _ipField.dispose();
    super.dispose();
  }

  ({String domain, String status, String ip}) get _filter =>
      (domain: _domain, status: _status, ip: _ip);

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final log = ref.watch(gatewayAccessLogProvider(_filter));
    final domains = {
      for (final r
          in ref.watch(gatewayRoutesProvider).value ?? const <GatewayRoute>[])
        r.domain,
    }.toList()..sort();
    if (_domain.isNotEmpty && !domains.contains(_domain)) {
      domains.insert(0, _domain);
    }
    return NCard(
      title: s.gatewayLatestRequests,
      trailing: IconButton(
        tooltip: s.actionRefresh,
        icon: const Icon(Icons.refresh_rounded, size: 18),
        onPressed: () => ref.invalidate(gatewayAccessLogProvider(_filter)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Wrap(
            spacing: 10,
            runSpacing: 10,
            children: [
              SizedBox(
                width: 240,
                child: DropdownButtonFormField<String>(
                  initialValue: _domain,
                  isExpanded: true,
                  decoration: InputDecoration(
                    labelText: s.gatewayDomain,
                    isDense: true,
                  ),
                  items: [
                    DropdownMenuItem(
                      value: '',
                      child: Text(s.gatewayAllDomains),
                    ),
                    for (final d in domains)
                      DropdownMenuItem(
                        value: d,
                        child: Text(d, overflow: TextOverflow.ellipsis),
                      ),
                  ],
                  onChanged: (v) => setState(() => _domain = v ?? ''),
                ),
              ),
              SizedBox(
                width: 150,
                child: DropdownButtonFormField<String>(
                  initialValue: _status,
                  decoration: InputDecoration(
                    labelText: s.gatewayStatus,
                    isDense: true,
                  ),
                  items: [
                    DropdownMenuItem(value: '', child: Text(s.gatewayAny)),
                    DropdownMenuItem(value: '2xx', child: Text('2xx')),
                    DropdownMenuItem(value: '3xx', child: Text('3xx')),
                    DropdownMenuItem(value: '4xx', child: Text('4xx')),
                    DropdownMenuItem(value: '5xx', child: Text('5xx')),
                  ],
                  onChanged: (v) => setState(() => _status = v ?? ''),
                ),
              ),
              SizedBox(
                width: 200,
                child: TextField(
                  controller: _ipField,
                  decoration: InputDecoration(
                    labelText: s.gatewayClientIp,
                    isDense: true,
                    suffixIcon: _ip.isEmpty
                        ? null
                        : IconButton(
                            icon: const Icon(Icons.close_rounded, size: 16),
                            onPressed: () => setState(() {
                              _ipField.clear();
                              _ip = '';
                            }),
                          ),
                  ),
                  onSubmitted: (v) => setState(() => _ip = v.trim()),
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          AsyncBlock(
            value: log,
            loadingHeight: 120,
            onRetry: () => ref.invalidate(gatewayAccessLogProvider(_filter)),
            data: (entries) {
              if (entries.isEmpty) {
                return Text(
                  s.gatewayNoRequestsMatchTheGateway,
                  style: TextStyle(color: c.muted, fontSize: 13),
                );
              }
              return Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (final e in entries) ...[
                    const Divider(height: 1),
                    Padding(
                      padding: const EdgeInsets.symmetric(vertical: 5),
                      child: Row(
                        children: [
                          SizedBox(
                            width: 64,
                            child: Text(
                              e.time == null ? '' : timeAgo(e.time),
                              style: TextStyle(color: c.muted, fontSize: 12),
                            ),
                          ),
                          SizedBox(
                            width: 40,
                            child: Text(
                              '${e.status}',
                              style: monoStyle(
                                context,
                                size: 12.5,
                                color: e.status >= 500
                                    ? c.failed
                                    : (e.status >= 400 ? c.warning : c.running),
                              ),
                            ),
                          ),
                          SizedBox(
                            width: 58,
                            child: Text(
                              e.method,
                              style: monoStyle(context, size: 12.5),
                            ),
                          ),
                          Expanded(
                            child: Text(
                              '${e.host}${e.path}',
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: monoStyle(context, size: 12.5),
                            ),
                          ),
                          SizedBox(
                            width: 64,
                            child: Text(
                              s.gatewayMs(e.ms),
                              textAlign: TextAlign.right,
                              style: TextStyle(color: c.muted, fontSize: 12),
                            ),
                          ),
                          SizedBox(
                            width: 128,
                            child: Align(
                              alignment: Alignment.centerRight,
                              child: InkWell(
                                onTap: () => setState(() {
                                  _ipField.text = e.ip;
                                  _ip = e.ip;
                                }),
                                child: Text(
                                  e.ip,
                                  style: TextStyle(
                                    color: c.muted,
                                    fontSize: 12,
                                  ),
                                ),
                              ),
                            ),
                          ),
                          if (widget.canBlock)
                            IconButton(
                              tooltip: s.gatewayBlock2(e.ip),
                              visualDensity: VisualDensity.compact,
                              icon: Icon(
                                Icons.block_rounded,
                                size: 16,
                                color: c.muted,
                              ),
                              onPressed: () => showDialog<void>(
                                context: context,
                                builder: (_) => _BlockDialog(ip: e.ip),
                              ),
                            ),
                        ],
                      ),
                    ),
                  ],
                ],
              );
            },
          ),
        ],
      ),
    );
  }
}
