import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/hex.dart';
import '../apps/deploy_progress.dart';
import '../auth/auth_frame.dart';

/// The sample app of the last onboarding step: a tiny web server that echoes the request.
const sampleImage = 'traefik/whoami:v1.10.1';

enum _Step { domain, https, sample, done }

/// First-run onboarding for the Owner: base domain + DNS check → Let's Encrypt email → a sample app.
/// Every step can be skipped; settings stay editable later.
class OnboardingPage extends ConsumerStatefulWidget {
  const OnboardingPage({super.key});

  @override
  ConsumerState<OnboardingPage> createState() => _OnboardingPageState();
}

class _OnboardingPageState extends ConsumerState<OnboardingPage> {
  _Step _step = _Step.domain;
  bool _loaded = false;
  bool _busy = false;
  String? _error;

  // domain
  final _base = TextEditingController();
  List<String> _serverIps = const [];
  List<String> _publicIps = const [];
  Map<String, dynamic>? _dns;

  // https
  final _email = TextEditingController();
  bool _gatewayOk = false;
  bool _ingress = false;

  // sample app
  App? _sample;
  Release? _sampleRelease;
  bool? _sampleOk;
  String? _sampleUrl;

  @override
  void dispose() {
    _base.dispose();
    _email.dispose();
    super.dispose();
  }

  void _load(Map<String, dynamic> d) {
    if (_loaded) return;
    _loaded = true;
    _base.text = (d['base_domain'] as String?) ?? '';
    _email.text =
        (d['acme_email'] as String?) ??
        (ref.read(sessionProvider).user?.email ?? '');
    if (_email.text.isEmpty) {
      _email.text = ref.read(sessionProvider).user?.email ?? '';
    }
    _serverIps = [
      for (final ip in (d['server_ips'] as List? ?? const [])) '$ip',
    ];
    _publicIps = [
      for (final ip in (d['public_ips'] as List? ?? const [])) '$ip',
    ];
    final gw = (d['gateway'] as Map?) ?? const {};
    _gatewayOk = gw['reachable'] == true;
    _ingress = gw['routing'] == 'ingress';
  }

  Future<void> _run(Future<void> Function() action) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await action();
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _checkDns() => _run(() async {
    final res = await ref
        .read(apiProvider)
        .post('/settings/dns-check', data: {'base_domain': _base.text.trim()});
    setState(() => _dns = res is Map ? Map<String, dynamic>.from(res) : null);
  });

  Future<void> _saveDomain() => _run(() async {
    await ref
        .read(apiProvider)
        .put(
          '/settings',
          data: {'base_domain': _base.text.trim(), 'public_ips': _publicIps},
        );
    ref.invalidate(panelSettingsProvider);
    setState(() => _step = _Step.https);
  });

  Future<void> _saveEmail() => _run(() async {
    await ref
        .read(apiProvider)
        .put('/gateway/settings', data: {'acme_email': _email.text.trim()});
    setState(() => _step = _Step.sample);
  });

  Future<void> _deploySample() => _run(() async {
    final api = ref.read(apiProvider);
    // reuse the "hello" project when it exists
    final projects = listOf(await api.get('/projects'), Project.fromJson);
    var project = projects.where((p) => p.slug == 'hello').firstOrNull;
    project ??= Project.fromJson(
      await api.post('/projects', data: {'slug': 'hello', 'name': 'Hello'}),
    );
    final apps = listOf(
      await api.get('/projects/${project.id}/apps'),
      App.fromJson,
    );
    var app = apps.where((a) => a.slug == 'world').firstOrNull;
    app ??= App.fromJson(
      await api.post(
        '/projects/${project.id}/apps',
        data: {
          'slug': 'world',
          'name': 'Hello world',
          'kind': 'web',
          'runtime': 'image',
          'config': {
            'image': sampleImage,
            'port': 80,
            'resources': {'cpu_millicores': 50, 'memory_mb': 64},
          },
        },
      ),
    );
    final rel = Release.fromJson(await api.post('/apps/${app.id}/deploy'));
    final domains = listOf(
      await api.get('/apps/${app.id}/domains'),
      Domain.fromJson,
    );
    ref.invalidate(projectsProvider);
    setState(() {
      _sample = app;
      _sampleRelease = rel;
      _sampleUrl = domains.where((d) => d.kind == 'app').firstOrNull?.url;
    });
  });

  Future<void> _finish() => _run(() async {
    await ref.read(apiProvider).put('/onboarding', data: {'done': true});
    ref.invalidate(onboardingProvider);
    if (mounted) context.go('/');
  });

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final state = ref.watch(onboardingProvider);
    final theme = Theme.of(context);
    final c = NectarColors.of(context);
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(20),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 620),
              child: state.when(
                loading: () => const LoadingBlock(height: 300),
                error: (e, _) => ErrorBlock(
                  error: e,
                  onRetry: () => ref.invalidate(onboardingProvider),
                ),
                data: (d) {
                  if (d == null) {
                    return EmptyState(
                      icon: Icons.lock_outline_rounded,
                      title: s.onboardingOnlyAnOwnerCanSet,
                      action: FilledButton(
                        onPressed: () => context.go('/'),
                        child: Text(s.goToDashboard),
                      ),
                    );
                  }
                  _load(d);
                  return Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      const Center(child: NectarMark(size: 52)),
                      const SizedBox(height: 14),
                      Text(
                        s.onboardingSetUpNectarApps,
                        textAlign: TextAlign.center,
                        style: theme.textTheme.headlineSmall,
                      ),
                      const SizedBox(height: 6),
                      Text(
                        s.onboardingThreeShortStepsYouCan,
                        textAlign: TextAlign.center,
                        style: TextStyle(color: c.muted),
                      ),
                      const SizedBox(height: 18),
                      _Dots(index: _step.index, count: _Step.values.length),
                      const SizedBox(height: 18),
                      Card(
                        child: Padding(
                          padding: const EdgeInsets.all(24),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              if (_error != null) FormError(_error!),
                              switch (_step) {
                                _Step.domain => _domainStep(),
                                _Step.https => _httpsStep(),
                                _Step.sample => _sampleStep(),
                                _Step.done => _doneStep(),
                              },
                            ],
                          ),
                        ),
                      ),
                      const SizedBox(height: 12),
                      if (_step != _Step.done)
                        TextButton(
                          onPressed: _busy ? null : _finish,
                          child: Text(s.onboardingSkipSetupForNow),
                        ),
                    ],
                  );
                },
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _title(String t, String sub) {
    final c = NectarColors.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(t, style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 4),
          Text(sub, style: TextStyle(color: c.muted, fontSize: 13.5)),
        ],
      ),
    );
  }

  Widget _domainStep() {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final base = _base.text.trim().isEmpty
        ? 'apps.example.com'
        : _base.text.trim();
    final ip = _publicIps.isNotEmpty
        ? _publicIps.first
        : (_serverIps.isNotEmpty ? _serverIps.first : '<your server IP>');
    final dns = _dns;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _title(
          'Your domain',
          'Every app gets an address like shop-myproject.$base, with HTTPS.',
        ),
        TextField(
          controller: _base,
          style: monoStyle(context, size: 14),
          decoration: InputDecoration(
            labelText: s.onboardingBaseDomain,
            hintText: 'apps.example.com',
          ),
          onChanged: (_) => setState(() => _dns = null),
        ),
        const SizedBox(height: 16),
        Text(
          s.onboardingAtYourDnsProviderAdd,
          style: TextStyle(color: c.muted, fontSize: 13),
        ),
        const SizedBox(height: 8),
        Container(
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: c.elevated,
            borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
            border: Border.all(color: c.border),
          ),
          child: SelectableText(
            '*.$base    A    $ip',
            style: monoStyle(context, size: 13.5),
          ),
        ),
        if (_serverIps.length > 1)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Text(
              'Server addresses: ${_serverIps.join(', ')}',
              style: TextStyle(color: c.muted, fontSize: 12),
            ),
          ),
        const SizedBox(height: 14),
        if (dns != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(
                  dns['ok'] == true
                      ? Icons.check_circle_rounded
                      : Icons.info_outline_rounded,
                  size: 18,
                  color: dns['ok'] == true ? c.running : c.warning,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    dns['ok'] == true
                        ? 'DNS is ready: *.$base points to this server.'
                        : '${dns['message']}. DNS changes can take a few minutes; you can continue and check later.',
                    style: const TextStyle(fontSize: 13.5),
                  ),
                ),
              ],
            ),
          ),
        Row(
          children: [
            OutlinedButton(
              onPressed: _busy || _base.text.trim().isEmpty ? null : _checkDns,
              child: Text(s.domainCheckDns),
            ),
            const Spacer(),
            SizedBox(
              width: 140,
              child: BusyButton(
                label: s.onboardingContinue,
                busy: _busy,
                onPressed: _base.text.trim().isEmpty ? null : _saveDomain,
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _httpsStep() {
    final l = S.of(context);
    final c = NectarColors.of(context);
    if (_ingress || !_gatewayOk) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _title(
            'HTTPS',
            _ingress
                ? 'Certificates come from your ingress controller and cert-manager on this server.'
                : 'The gateway is not reachable right now, so the certificate email cannot be set here. You can set it later in Settings.',
          ),
          Align(
            alignment: Alignment.centerRight,
            child: FilledButton(
              onPressed: () => setState(() => _step = _Step.sample),
              child: Text(l.onboardingContinue),
            ),
          ),
        ],
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _title(
          'HTTPS certificates',
          'Free certificates from Let\'s Encrypt, renewed automatically.',
        ),
        TextField(
          controller: _email,
          keyboardType: TextInputType.emailAddress,
          decoration: InputDecoration(
            labelText: l.onboardingEmailForLetSEncrypt,
            helperText: l.onboardingOnlyUsedForExpiryWarnings,
          ),
        ),
        const SizedBox(height: 18),
        Row(
          children: [
            TextButton(
              onPressed: () => setState(() => _step = _Step.domain),
              child: Text(l.projectBack),
            ),
            const Spacer(),
            SizedBox(
              width: 140,
              child: BusyButton(
                label: l.onboardingContinue,
                busy: _busy,
                onPressed: _saveEmail,
              ),
            ),
          ],
        ),
        Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Text(
            l.onboardingLeaveItEmptyToRegister,
            style: TextStyle(color: c.muted, fontSize: 12),
          ),
        ),
      ],
    );
  }

  Widget _sampleStep() {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final app = _sample;
    final rel = _sampleRelease;
    if (app != null && rel != null) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _title(
            'Deploying "Hello world"',
            'A tiny web server in the project "Hello".',
          ),
          DeployProgress(
            app: app,
            releaseId: rel.id,
            onFinished: (ok) => setState(() => _sampleOk = ok),
          ),
          const SizedBox(height: 16),
          if (_sampleOk == true && _sampleUrl != null)
            OutlinedButton.icon(
              onPressed: () => launchUrl(Uri.parse(_sampleUrl!)),
              icon: const Icon(Icons.open_in_new_rounded, size: 18),
              label: Text(_sampleUrl!),
            ),
          if (_sampleOk != null)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: Align(
                alignment: Alignment.centerRight,
                child: FilledButton(
                  onPressed: () => setState(() => _step = _Step.done),
                  child: Text(s.onboardingContinue),
                ),
              ),
            ),
        ],
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _title(
          'Try it with a sample app',
          'Deploys a small "Hello world" web app so you can see the whole path: image, start, health check, domain.',
        ),
        Text(
          s.onboardingImage50mCpu64Mb(sampleImage),
          style: monoStyle(context, size: 12.5, color: c.muted),
        ),
        const SizedBox(height: 18),
        Row(
          children: [
            TextButton(
              onPressed: () => setState(() => _step = _Step.done),
              child: Text(s.onboardingSkip),
            ),
            const Spacer(),
            SizedBox(
              width: 170,
              child: BusyButton(
                label: s.onboardingDeploySample,
                busy: _busy,
                onPressed: _deploySample,
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _doneStep() => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      _title(
        'All set',
        'Create a project and deploy your first app: a JAR, a binary, a static site, Node, Python, PHP or any Docker image.',
      ),
      Wrap(
        spacing: 10,
        runSpacing: 10,
        alignment: WrapAlignment.end,
        children: [
          OutlinedButton(
            onPressed: _busy ? null : _finish,
            child: const Text('Go to the dashboard'),
          ),
          FilledButton.icon(
            onPressed: _busy
                ? null
                : () async {
                    await _finish();
                    if (mounted) context.go('/apps/new');
                  },
            icon: const Icon(Icons.add_rounded, size: 18),
            label: const Text('New app'),
          ),
        ],
      ),
    ],
  );
}

class _Dots extends StatelessWidget {
  const _Dots({required this.index, required this.count});

  final int index;
  final int count;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        for (var i = 0; i < count; i++)
          AnimatedContainer(
            duration: NectarTokens.fast,
            margin: const EdgeInsets.symmetric(horizontal: 4),
            width: i == index ? 22 : 8,
            height: 8,
            decoration: BoxDecoration(
              color: i <= index ? NectarTokens.amber : c.border,
              borderRadius: BorderRadius.circular(99),
            ),
          ),
      ],
    );
  }
}
