import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
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

/// Settings → API tokens: tokens for CI and scripts (the nectar CLI, the GitHub Action, curl).
class TokensPage extends ConsumerWidget {
  const TokensPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final session = ref.watch(sessionProvider);
    final mine = ref.watch(myTokensProvider);
    return PageScaffold(
      title: s.acceApiTokens,
      subtitle: s.acceForCiAndScriptsA,
      maxWidth: 980,
      onRefresh: () async {
        ref.invalidate(myTokensProvider);
        ref.invalidate(allTokensProvider);
      },
      actions: [
        FilledButton.icon(
          onPressed: () => showDialog<void>(
            context: context,
            builder: (_) => const _NewTokenDialog(),
          ),
          icon: const Icon(Icons.key_rounded, size: 18),
          label: Text(s.acceNewToken),
        ),
      ],
      children: [
        NCard(
          title: s.acceYourTokens,
          child: AsyncBlock(
            value: mine,
            onRetry: () => ref.invalidate(myTokensProvider),
            data: (list) => list.isEmpty
                ? EmptyState(
                    icon: Icons.key_off_rounded,
                    title: s.acceNoTokensYet,
                    message: s.acceCreateOneForGithubActions,
                  )
                : _TokenList(tokens: list, own: true),
          ),
        ),
        const SizedBox(height: NectarTokens.gap),
        const _UsageCard(),
        if (session.can('token.manage')) ...[
          const SizedBox(height: NectarTokens.gap),
          NCard(
            title: s.acceEveryoneSTokens,
            child: AsyncBlock(
              value: ref.watch(allTokensProvider),
              onRetry: () => ref.invalidate(allTokensProvider),
              data: (list) => list.isEmpty
                  ? Text(s.acceNoTokens)
                  : _TokenList(tokens: list, own: false),
            ),
          ),
        ],
      ],
    );
  }
}

class _TokenList extends ConsumerWidget {
  const _TokenList({required this.tokens, required this.own});

  final List<ApiTokenInfo> tokens;
  final bool own;

  Future<void> _revoke(
    BuildContext context,
    WidgetRef ref,
    ApiTokenInfo t,
  ) async {
    final s = S.of(context);
    if (!await confirmAction(
      context,
      s.acceRevoke(t.name),
      s.acceScriptsAndPipelinesThatUse,
      s.acceRevoke2,
      danger: true,
    )) {
      return;
    }
    try {
      await ref
          .read(apiProvider)
          .delete(own ? '/me/tokens/${t.id}' : '/tokens/${t.id}');
      ref.invalidate(myTokensProvider);
      ref.invalidate(allTokensProvider);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return Column(
      children: [
        for (final t in tokens)
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: Icon(
              Icons.key_rounded,
              color: t.expired ? c.failed : NectarTokens.amber,
            ),
            title: Text(
              t.name,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            subtitle: Text(
              [
                if (!own) t.userEmail,
                t.roleName.isEmpty ? 'your rights' : t.roleName,
                t.projectName.isEmpty ? 'all projects' : t.projectName,
                t.lastUsedAt == null
                    ? 'never used'
                    : 'used ${timeAgo(t.lastUsedAt)}',
                if (t.expiresAt != null)
                  t.expired
                      ? 'expired'
                      : 'expires ${t.expiresAt!.toIso8601String().substring(0, 10)}',
              ].join(' · '),
              style: TextStyle(
                fontSize: 12.5,
                color: t.expired ? c.failed : c.muted,
              ),
            ),
            trailing: IconButton(
              tooltip: s.acceRevoke2,
              icon: const Icon(Icons.delete_outline_rounded),
              onPressed: () => _revoke(context, ref, t),
            ),
          ),
      ],
    );
  }
}

class _UsageCard extends ConsumerWidget {
  const _UsageCard();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final c = NectarColors.of(context);
    final url =
        ref.watch(sessionProvider.select((s) => s.serverUrl)) ??
        'https://panel.example.com';
    final snippet =
        'nectar login --url $url\nnectar deploy shop/api --file target/app.jar\n\n'
        '# or in GitHub Actions (examples/github-actions in the repository):\n'
        '- uses: minelifes/nectar_apps@v0\n  with:\n    url: \${{ secrets.NECTAR_URL }}\n    token: \${{ secrets.NECTAR_TOKEN }}\n    app: shop/api\n    file: target/app.jar';
    return NCard(
      title: l.acceUsingAToken,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(
              color: const Color(0xFF0A0B0D),
              borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
            ),
            child: SelectableText(
              snippet,
              style: monoStyle(
                context,
                size: 12,
                color: const Color(0xFFD9DBE0),
              ),
            ),
          ),
          const SizedBox(height: 8),
          Text(
            l.acceAnyHttpClientWorksToo,
            style: TextStyle(color: c.muted, fontSize: 12.5),
          ),
        ],
      ),
    );
  }
}

class _NewTokenDialog extends ConsumerStatefulWidget {
  const _NewTokenDialog();

  @override
  ConsumerState<_NewTokenDialog> createState() => _NewTokenDialogState();
}

class _NewTokenDialogState extends ConsumerState<_NewTokenDialog> {
  final _name = TextEditingController();
  int _role = 0; // 0 = your rights
  int _project = 0; // 0 = all
  int _days = 90;
  bool _busy = false;
  String? _token;

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _create() async {
    setState(() => _busy = true);
    try {
      final res = await ref
          .read(apiProvider)
          .post(
            '/me/tokens',
            data: {
              'name': _name.text.trim(),
              if (_role != 0) 'role_id': _role,
              if (_project != 0) 'project_id': _project,
              'expires_in_days': _days,
            },
          );
      ref.invalidate(myTokensProvider);
      ref.invalidate(allTokensProvider);
      if (mounted) setState(() => _token = '${(res as Map)['token']}');
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
    if (_token != null) {
      return AlertDialog(
        title: Text(s.acceCopyYourToken),
        content: SizedBox(
          width: 560,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(s.acceItIsShownOnlyNow),
              const SizedBox(height: 10),
              Container(
                padding: const EdgeInsets.all(10),
                decoration: BoxDecoration(
                  color: c.elevated,
                  borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
                ),
                child: SelectableText(
                  _token!,
                  style: monoStyle(context, size: 13),
                ),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () {
              Clipboard.setData(ClipboardData(text: _token!));
              showSnack(context, s.backupCopied);
            },
            child: Text(s.backupCopy),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context),
            child: Text(s.backupDone),
          ),
        ],
      );
    }
    final roles = ref.watch(rolesProvider);
    final projects = ref.watch(projectsProvider);
    return AlertDialog(
      title: Text(s.acceNewApiToken),
      content: SizedBox(
        width: 480,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextField(
              controller: _name,
              decoration: InputDecoration(
                labelText: s.backupName,
                hintText: s.acceGithubActionsShop,
              ),
            ),
            const SizedBox(height: 10),
            DropdownButtonFormField<int>(
              initialValue: _role,
              isExpanded: true,
              decoration: InputDecoration(labelText: s.acceRights),
              items: [
                DropdownMenuItem(
                  value: 0,
                  child: Text(s.acceEverythingYouMayDo),
                ),
                for (final r in roles.value ?? const <RoleInfo>[])
                  DropdownMenuItem(
                    value: r.id,
                    child: Text(s.acceOnly(r.name)),
                  ),
              ],
              onChanged: (v) => setState(() => _role = v ?? 0),
            ),
            const SizedBox(height: 10),
            DropdownButtonFormField<int>(
              initialValue: _project,
              isExpanded: true,
              decoration: InputDecoration(labelText: s.navProjects),
              items: [
                DropdownMenuItem(value: 0, child: Text(s.projectAllProjects)),
                for (final p in projects.value ?? const <Project>[])
                  DropdownMenuItem(
                    value: p.id,
                    child: Text(s.acceOnly2(p.name)),
                  ),
              ],
              onChanged: (v) => setState(() => _project = v ?? 0),
            ),
            const SizedBox(height: 10),
            DropdownButtonFormField<int>(
              initialValue: _days,
              decoration: InputDecoration(labelText: s.acceExpires),
              items: [
                DropdownMenuItem(value: 30, child: Text(s.acceIn30Days)),
                DropdownMenuItem(value: 90, child: Text(s.acceIn90Days)),
                DropdownMenuItem(value: 365, child: Text(s.acceInAYear)),
                DropdownMenuItem(value: 0, child: Text('never')),
              ],
              onChanged: (v) => setState(() => _days = v ?? 90),
            ),
            const SizedBox(height: 8),
            Text(
              s.acceATokenNeverHasMore,
              style: TextStyle(color: c.muted, fontSize: 12.5),
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
          child: Text(_busy ? 'Creating…' : 'Create'),
        ),
      ],
    );
  }
}
