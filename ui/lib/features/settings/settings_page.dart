import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/session.dart';
import '../../core/locale.dart';
import '../../core/theme_mode.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/hex.dart';
import 'registries_card.dart';
import 'security_cards.dart';

class SettingsPage extends ConsumerWidget {
  const SettingsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    final mode = ref.watch(themeModeProvider);
    final settings = ref.watch(panelSettingsProvider);
    final user = session.user;
    final theme = Theme.of(context);

    return PageScaffold(
      title: 'Settings',
      maxWidth: 820,
      children: [
        if (user != null)
          NCard(
            title: 'Account',
            child: Row(
              children: [
                HexBadge(
                  size: 48,
                  child: Text(user.initials, style: const TextStyle(fontWeight: FontWeight.w700, color: NectarTokens.onAmber)),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(user.name, style: theme.textTheme.titleMedium),
                      Text(user.email, style: theme.textTheme.bodySmall),
                      if (user.isOwner)
                        Padding(
                          padding: const EdgeInsets.only(top: 4),
                          child: Text('Owner', style: TextStyle(color: NectarTokens.amber, fontSize: 12, fontWeight: FontWeight.w600)),
                        ),
                    ],
                  ),
                ),
                OutlinedButton.icon(
                  onPressed: () => ref.read(sessionProvider.notifier).logout(),
                  icon: const Icon(Icons.logout_rounded, size: 18),
                  label: const Text('Sign out'),
                ),
              ],
            ),
          ),
        if (user != null) ...[
          const SizedBox(height: NectarTokens.gap),
          const TwoFactorCard(),
          const SizedBox(height: NectarTokens.gap),
          const SessionsCard(),
        ],
        NCard(
          padding: const EdgeInsets.symmetric(vertical: 4),
          child: Column(
            children: [
              if (session.can('user.manage'))
                ListTile(
                  leading: const Icon(Icons.people_alt_outlined),
                  title: const Text('Users'),
                  subtitle: const Text('Invite people and give them roles, everywhere or per project'),
                  trailing: const Icon(Icons.chevron_right_rounded),
                  onTap: () => context.go('/users'),
                ),
              ListTile(
                leading: const Icon(Icons.admin_panel_settings_outlined),
                title: const Text('Roles'),
                subtitle: const Text('Permission sets; clone a built-in role to make your own'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go('/roles'),
              ),
              ListTile(
                leading: const Icon(Icons.key_rounded),
                title: const Text('API tokens'),
                subtitle: const Text('For the nectar CLI, GitHub Actions and scripts'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go('/tokens'),
              ),
            ],
          ),
        ),
        const SizedBox(height: NectarTokens.gap),
        if (session.can('backup.manage') || session.can('server.view')) ...[
          const SizedBox(height: NectarTokens.gap),
          NCard(
            padding: const EdgeInsets.symmetric(vertical: 4),
            child: Column(
              children: [
                if (session.can('backup.manage'))
                  ListTile(
                    leading: const Icon(Icons.backup_outlined),
                    title: const Text('Backups'),
                    subtitle: const Text('Plans, targets, restore and the recovery kit'),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () => context.go('/backups'),
                  ),
                if (session.can('server.view'))
                  ListTile(
                    leading: const Icon(Icons.system_update_alt_rounded),
                    title: const Text('Updates'),
                    subtitle: const Text('Panel, gateway and k3s versions, automatic updates, history and rollback'),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () => context.go('/updates'),
                  ),
                if (session.can('server.view'))
                  ListTile(
                    leading: const Icon(Icons.notifications_outlined),
                    title: const Text('Alerts'),
                    subtitle: const Text('Rules and notification channels'),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () => context.go('/alerts'),
                  ),
              ],
            ),
          ),
        ],
        const SizedBox(height: NectarTokens.gap),
        NCard(
          title: context.l10n.settingsAppearance,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              SegmentedButton<ThemeMode>(
                segments: [
                  ButtonSegment(value: ThemeMode.dark, icon: const Icon(Icons.dark_mode_outlined, size: 18), label: Text(context.l10n.themeDark)),
                  ButtonSegment(value: ThemeMode.light, icon: const Icon(Icons.light_mode_outlined, size: 18), label: Text(context.l10n.themeLight)),
                  ButtonSegment(value: ThemeMode.system, icon: const Icon(Icons.brightness_auto_outlined, size: 18), label: Text(context.l10n.themeSystem)),
                ],
                selected: {mode},
                showSelectedIcon: false,
                onSelectionChanged: (s) => ref.read(themeModeProvider.notifier).set(s.first),
              ),
              const SizedBox(height: 14),
              Text(context.l10n.settingsLanguage, style: Theme.of(context).textTheme.titleSmall),
              const SizedBox(height: 6),
              SegmentedButton<String>(
                segments: [
                  ButtonSegment(value: '', label: Text(context.l10n.languageSystem)),
                  ButtonSegment(value: 'en', label: Text(context.l10n.languageEnglish)),
                  ButtonSegment(value: 'uk', label: Text(context.l10n.languageUkrainian)),
                ],
                selected: {ref.watch(localeProvider)?.languageCode ?? ''},
                showSelectedIcon: false,
                onSelectionChanged: (s) => ref.read(localeProvider.notifier).set(s.first.isEmpty ? null : Locale(s.first)),
              ),
              const SizedBox(height: 6),
              Text(context.l10n.settingsLanguageHint, style: TextStyle(color: NectarColors.of(context).muted, fontSize: 12.5)),
            ],
          ),
        ),
        const SizedBox(height: NectarTokens.gap),
        NCard(
          title: 'Server',
          trailing: session.canChangeServer
              ? TextButton(
                  onPressed: () => ref.read(sessionProvider.notifier).disconnect(),
                  child: const Text('Use another server'),
                )
              : null,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _Row('Address', session.serverUrl ?? '', mono: true),
              if (session.version != null) _Row('Version', '${session.version!.version} (API v${session.version!.apiVersion})'),
              settings.when(
                skipLoadingOnRefresh: true,
                data: (s) => Column(
                  children: [
                    _Row('Base domain', s.baseDomain.isEmpty ? 'not set' : s.baseDomain, mono: s.baseDomain.isNotEmpty),
                    _Row(
                      'Domains via',
                      s.routing == 'ingress'
                          ? 'Kubernetes Ingress'
                          : (!s.gatewayConfigured ? 'nectar-gateway (not configured)' : (s.gatewayReachable ? 'nectar-gateway' : 'nectar-gateway (unreachable)')),
                    ),
                  ],
                ),
                loading: () => const SizedBox.shrink(),
                error: (e, _) => const SizedBox.shrink(),
              ),
              if (session.can('settings.manage'))
                Align(
                  alignment: Alignment.centerLeft,
                  child: TextButton.icon(
                    onPressed: () => context.go('/welcome'),
                    icon: const Icon(Icons.tune_rounded, size: 18),
                    label: const Text('Domain, HTTPS email and sample app (setup wizard)'),
                  ),
                ),
            ],
          ),
        ),
        if (session.can('settings.manage')) ...[
          const SizedBox(height: NectarTokens.gap),
          const SmtpCard(),
          const SizedBox(height: NectarTokens.gap),
          const AccessCard(),
          const SizedBox(height: NectarTokens.gap),
          const RegistriesCard(),
        ],
        const SizedBox(height: NectarTokens.gap),
        NCard(
          title: 'About',
          child: Row(
            children: [
              const NectarMark(size: 34),
              const SizedBox(width: 12),
              const Expanded(child: Text('Nectar Apps · © TechBrain LLC · Nectar Source License')),
              TextButton(
                onPressed: () => showLicensePage(
                  context: context,
                  applicationName: 'Nectar Apps',
                  applicationVersion: session.version?.version,
                  applicationIcon: const Padding(padding: EdgeInsets.all(12), child: NectarMark(size: 48)),
                ),
                child: const Text('Licenses'),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _Row extends StatelessWidget {
  const _Row(this.label, this.value, {this.mono = false});

  final String label;
  final String value;
  final bool mono;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(width: 130, child: Text(label, style: TextStyle(color: c.muted, fontSize: 13.5))),
          Expanded(child: SelectableText(value, style: mono ? monoStyle(context, size: 13) : const TextStyle(fontSize: 13.5))),
        ],
      ),
    );
  }
}
