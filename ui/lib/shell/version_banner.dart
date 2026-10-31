import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/session.dart';
import '../core/client_version.dart';
import '../data/providers.dart';
import '../generated/l10n.dart';
import '../theme/tokens.dart';

/// Web: the panel was updated while this tab was open → offer a reload (the UI is embedded in the panel).
/// Apps: the server needs a newer app → ask to update it from the store.
class VersionBanner extends ConsumerStatefulWidget {
  const VersionBanner({super.key});

  @override
  ConsumerState<VersionBanner> createState() => _VersionBannerState();
}

class _VersionBannerState extends ConsumerState<VersionBanner> {
  String? _dismissed;

  @override
  Widget build(BuildContext context) {
    final l = S.of(context);
    final loaded = ref.watch(sessionProvider.select((s) => s.version));
    final latest = ref.watch(serverVersionProvider).value;
    if (loaded == null || latest == null) return const SizedBox.shrink();
    final c = NectarColors.of(context);

    if (!kIsWeb && latest.minClientVersion.isNotEmpty && compareVersions(clientVersion, latest.minClientVersion) < 0) {
      return _bar(
        context,
        color: c.failed,
        icon: Icons.system_update_rounded,
        text: 'This app ($clientVersion) is too old for the server: version ${latest.minClientVersion} or newer is needed. '
            'Please update it from the App Store or Google Play.',
      );
    }
    // a server clock that is off makes every time in the panel wrong ("17h ago"), breaks 2FA codes and TLS
    final skew = latest.clockSkew;
    if (skew != null && skew.abs() > const Duration(minutes: 2) && _dismissed != 'clock') {
      final a = skew.abs();
      final amount = a.inHours > 0 ? '${a.inHours} h ${a.inMinutes % 60} min' : '${a.inMinutes} min';
      return _bar(
        context,
        color: c.warning,
        icon: Icons.schedule_rounded,
        text: skew.isNegative ? l.shellClockBehind(amount) : l.shellClockAhead(amount),
        actions: [TextButton(onPressed: () => setState(() => _dismissed = 'clock'), child: Text(l.shellLater))],
      );
    }
    final changed = latest.version != loaded.version || (latest.commit.isNotEmpty && loaded.commit.isNotEmpty && latest.commit != loaded.commit);
    if (kIsWeb && changed && _dismissed != latest.version) {
      return _bar(
        context,
        color: NectarTokens.amber,
        icon: Icons.auto_awesome_rounded,
        text: 'Nectar Apps was updated to ${latest.version}. Reload to use the new version.',
        actions: [
          TextButton(onPressed: () => setState(() => _dismissed = latest.version), child: Text(l.shellLater)),
          FilledButton(
            onPressed: () => launchUrl(Uri.base, webOnlyWindowName: '_self'),
            child: Text(l.shellReload),
          ),
        ],
      );
    }
    return const SizedBox.shrink();
  }

  Widget _bar(BuildContext context, {required Color color, required IconData icon, required String text, List<Widget> actions = const []}) {
    return Material(
      color: color.withValues(alpha: 0.14),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        child: Row(
          children: [
            Icon(icon, color: color, size: 20),
            const SizedBox(width: 10),
            Expanded(child: Text(text, style: const TextStyle(fontSize: 13.5))),
            for (final a in actions) Padding(padding: const EdgeInsets.only(left: 8), child: a),
          ],
        ),
      ),
    );
  }
}
