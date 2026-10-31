import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

/// A configured sign-in provider as GET /settings/sso returns it.
class SsoProvider {
  SsoProvider(
    Map<String, dynamic> j, {
    this.hasSecret = false,
    this.callbackUrl = '',
  }) : id = '${j['id'] ?? ''}',
       kind = '${j['kind'] ?? 'oidc'}',
       name = '${j['name'] ?? ''}',
       clientId = '${j['client_id'] ?? ''}',
       issuer = '${j['issuer'] ?? ''}',
       allowedDomains = [
         for (final d in (j['allowed_domains'] as List?) ?? const []) '$d',
       ],
       autoCreate = j['auto_create'] == true,
       defaultRole = '${j['default_role'] ?? ''}',
       groupRoles = {
         for (final e in ((j['group_roles'] as Map?) ?? const {}).entries)
           '${e.key}': '${e.value}',
       },
       enabled = j['enabled'] != false;

  String id, kind, name, clientId, issuer, defaultRole;
  List<String> allowedDomains;
  bool autoCreate, enabled, hasSecret;
  Map<String, String> groupRoles;
  String callbackUrl;
  String secret = ''; // new client secret ('' keeps the stored one)

  Map<String, dynamic> toJson() => {
    'id': id,
    'kind': kind,
    'name': name,
    'client_id': clientId,
    if (kind == 'oidc') 'issuer': issuer,
    'allowed_domains': allowedDomains,
    'auto_create': autoCreate,
    'default_role': defaultRole,
    'group_roles': groupRoles,
    'enabled': enabled,
    if (secret.isNotEmpty) 'client_secret': secret,
  };
}

class SsoSettings {
  const SsoSettings(this.providers, this.callbackBase);
  final List<SsoProvider> providers;
  final String callbackBase;
}

SsoSettings _parse(Object? v) {
  final m = v is Map ? v : const {};
  return SsoSettings([
    for (final p in ((m['providers'] as List?) ?? const []).whereType<Map>())
      SsoProvider(
        Map<String, dynamic>.from(p['provider'] as Map? ?? const {}),
        hasSecret: p['has_secret'] == true,
        callbackUrl: '${p['callback_url'] ?? ''}',
      ),
  ], '${m['callback_base'] ?? ''}');
}

final ssoSettingsProvider = FutureProvider.autoDispose<SsoSettings>(
  (ref) async => _parse(await ref.watch(apiProvider).get('/settings/sso')),
);

/// Settings → Sign-in providers: GitHub, Google or any OpenID Connect provider as buttons on the login page.
class SsoCard extends ConsumerWidget {
  const SsoCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final c = NectarColors.of(context);
    final data = ref.watch(ssoSettingsProvider);
    return NCard(
      title: l.settingSignInProviders,
      trailing: data.hasValue
          ? TextButton.icon(
              onPressed: () => _edit(context, data.value!, null),
              icon: const Icon(Icons.add_rounded, size: 18),
              label: Text(l.settingAddProvider),
            )
          : null,
      child: AsyncBlock(
        value: data,
        loadingHeight: 60,
        onRetry: () => ref.invalidate(ssoSettingsProvider),
        data: (s) => s.providers.isEmpty
            ? Text(
                l.settingLetPeopleSignInWith,
                style: TextStyle(color: c.muted, fontSize: 12.5),
              )
            : Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (final p in s.providers)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      dense: true,
                      leading: Icon(
                        p.enabled ? Icons.login_rounded : Icons.block_rounded,
                        color: p.enabled ? c.running : c.muted,
                        size: 20,
                      ),
                      title: Text('${p.name} · ${p.kind}'),
                      subtitle: Text(
                        [
                          p.allowedDomains.isEmpty
                              ? 'existing users'
                              : p.allowedDomains.map((d) => '@$d').join(', '),
                          if (p.autoCreate) 'creates users',
                          if (!p.hasSecret) 'no client secret',
                        ].join(' · '),
                        style: TextStyle(color: c.muted, fontSize: 12),
                      ),
                      onTap: () => _edit(context, s, p),
                    ),
                ],
              ),
      ),
    );
  }

  void _edit(BuildContext context, SsoSettings s, SsoProvider? p) =>
      showDialog<void>(
        context: context,
        builder: (_) => _SsoDialog(settings: s, provider: p),
      );
}

class _SsoDialog extends ConsumerStatefulWidget {
  const _SsoDialog({required this.settings, required this.provider});

  final SsoSettings settings;
  final SsoProvider? provider;

  @override
  ConsumerState<_SsoDialog> createState() => _SsoDialogState();
}

class _SsoDialogState extends ConsumerState<_SsoDialog> {
  late final SsoProvider _p = SsoProvider(
    widget.provider?.toJson() ?? {'kind': 'oidc'},
    hasSecret: widget.provider?.hasSecret ?? false,
  );
  late final _id = TextEditingController(text: _p.id);
  late final _name = TextEditingController(text: _p.name);
  late final _clientId = TextEditingController(text: _p.clientId);
  final _secret = TextEditingController();
  late final _issuer = TextEditingController(text: _p.issuer);
  late final _domains = TextEditingController(
    text: _p.allowedDomains.join(', '),
  );
  late final _groups = TextEditingController(
    text: _p.groupRoles.entries.map((e) => '${e.key}=${e.value}').join('\n'),
  );
  bool _busy = false;

  bool get _isNew => widget.provider == null;

  @override
  void dispose() {
    for (final t in [
      _id,
      _name,
      _clientId,
      _secret,
      _issuer,
      _domains,
      _groups,
    ]) {
      t.dispose();
    }
    super.dispose();
  }

  Future<void> _save({bool delete = false}) async {
    final l = S.of(context);
    if (delete &&
        !await confirmAction(
          context,
          l.settingRemove(_p.name),
          l.settingItsButtonDisappearsFromThe,
          l.backupRemove2,
          danger: true,
        )) {
      return;
    }
    _p
      ..id = _id.text.trim()
      ..name = _name.text.trim()
      ..clientId = _clientId.text.trim()
      ..secret = _secret.text.trim()
      ..issuer = _issuer.text.trim()
      ..allowedDomains = [
        for (final d in _domains.text.split(RegExp(r'[,\s]+')))
          if (d.trim().isNotEmpty) d.trim(),
      ]
      ..groupRoles = {
        for (final line in _groups.text.split('\n'))
          if (line.contains('='))
            line.substring(0, line.indexOf('=')).trim(): line
                .substring(line.indexOf('=') + 1)
                .trim(),
      };
    final others = [
      for (final o in widget.settings.providers)
        if (o.id != widget.provider?.id) o,
    ];
    final list = [...others, if (!delete) _p];
    setState(() => _busy = true);
    try {
      await ref
          .read(apiProvider)
          .put('/settings/sso', data: [for (final o in list) o.toJson()]);
      ref.invalidate(ssoSettingsProvider);
      if (mounted) {
        Navigator.pop(context);
        showSnack(context, delete ? 'Removed' : l.projectSaved);
      }
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
    final roles = ref.watch(rolesProvider).value ?? const [];
    final callback = _isNew
        ? '${widget.settings.callbackBase}${_id.text.isEmpty ? '<id>' : _id.text}/callback'
        : widget.provider!.callbackUrl;
    return AlertDialog(
      title: Text(_isNew ? 'Add sign-in provider' : _p.name),
      content: SizedBox(
        width: 560,
        child: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              SegmentedButton<String>(
                segments: const [
                  ButtonSegment(value: 'oidc', label: Text('OpenID Connect')),
                  ButtonSegment(value: 'github', label: Text('GitHub')),
                  ButtonSegment(value: 'google', label: Text('Google')),
                ],
                selected: {_p.kind},
                onSelectionChanged: (v) => setState(() => _p.kind = v.first),
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _id,
                      enabled: _isNew,
                      decoration: InputDecoration(
                        labelText: s.settingId,
                        hintText: 'corp',
                      ),
                      onChanged: (_) => setState(() {}),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    flex: 2,
                    child: TextField(
                      controller: _name,
                      decoration: InputDecoration(
                        labelText: s.settingButtonLabel,
                        hintText: s.settingCompanySso,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 10),
              Text(
                s.settingCallbackRedirectUrlToRegister,
                style: TextStyle(color: c.muted, fontSize: 12.5),
              ),
              Row(
                children: [
                  Expanded(
                    child: SelectableText(
                      callback,
                      style: monoStyle(context, size: 12.5),
                    ),
                  ),
                  IconButton(
                    tooltip: s.backupCopy,
                    icon: const Icon(Icons.copy_rounded, size: 18),
                    onPressed: () =>
                        Clipboard.setData(ClipboardData(text: callback)),
                  ),
                ],
              ),
              if (_p.kind == 'oidc') ...[
                const SizedBox(height: 6),
                TextField(
                  controller: _issuer,
                  decoration: InputDecoration(
                    labelText: s.settingIssuerUrl,
                    hintText: 'https://auth.example.com/application/o/nectar/',
                  ),
                ),
              ],
              const SizedBox(height: 10),
              TextField(
                controller: _clientId,
                decoration: InputDecoration(labelText: s.settingClientId),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: _secret,
                obscureText: true,
                decoration: InputDecoration(
                  labelText: s.settingClientSecret,
                  hintText: _p.hasSecret
                      ? 'Stored — leave empty to keep it'
                      : null,
                ),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: _domains,
                decoration: InputDecoration(
                  labelText: s.settingAllowedEmailDomains,
                  hintText: s.settingExampleComExampleOrg,
                  helperText: s.settingEmptyOnlyUsersThatAlready,
                ),
              ),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(s.settingCreateUsersOnTheirFirst),
                subtitle: Text(s.settingNeedsAllowedDomains),
                value: _p.autoCreate,
                onChanged: (v) => setState(() => _p.autoCreate = v),
              ),
              if (_p.autoCreate) ...[
                DropdownButtonFormField<String>(
                  initialValue:
                      roles.any(
                        (r) =>
                            '${r.id}' == _p.defaultRole ||
                            r.builtinKey == _p.defaultRole,
                      )
                      ? _p.defaultRole
                      : '',
                  decoration: InputDecoration(
                    labelText: s.settingRoleOfNewUsersEverywhere,
                  ),
                  items: [
                    DropdownMenuItem(
                      value: '',
                      child: Text(s.settingNoneAnAdminAssignsRoles),
                    ),
                    for (final r in roles)
                      DropdownMenuItem(
                        value: r.builtin ? r.builtinKey : '${r.id}',
                        child: Text(r.name),
                      ),
                  ],
                  onChanged: (v) => setState(() => _p.defaultRole = v ?? ''),
                ),
                if (_p.kind == 'oidc') ...[
                  const SizedBox(height: 10),
                  TextField(
                    controller: _groups,
                    minLines: 2,
                    maxLines: 5,
                    style: monoStyle(context, size: 13),
                    decoration: InputDecoration(
                      labelText: s.settingGroupRoleOnePerLine,
                      hintText: s.settingNectarAdminsAdminDevelopersDeveloper,
                      helperText: s.settingAppliedAtEverySignIn,
                    ),
                  ),
                ],
              ],
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(s.settingShowOnTheLoginPage),
                value: _p.enabled,
                onChanged: (v) => setState(() => _p.enabled = v),
              ),
            ],
          ),
        ),
      ),
      actions: [
        if (!_isNew)
          TextButton(
            onPressed: _busy ? null : () => _save(delete: true),
            style: TextButton.styleFrom(foregroundColor: c.failed),
            child: Text(s.backupRemove2),
          ),
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: _busy ? null : _save,
          child: Text(_busy ? 'Saving…' : 'Save'),
        ),
      ],
    );
  }
}
