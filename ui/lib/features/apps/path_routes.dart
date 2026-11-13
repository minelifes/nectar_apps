// import 'package:flutter/material.dart';
// import 'package:flutter_riverpod/flutter_riverpod.dart';
// import 'package:go_router/go_router.dart';
//
// import '../../api/client.dart';
// import '../../api/models.dart';
// import '../../api/session.dart';
// import '../../data/providers.dart';
// import '../../generated/l10n.dart';
// import '../../theme/theme.dart';
// import '../../widgets/common.dart';
//
// // Paths of an app's domains that go to other apps: app1.com/api → app2 (a web app or a worker with a
// // port). Each is a domain of the other app with that host and path; the gateway takes the longest path.
//
// /// Hosts an app serves as a whole (its own domains on "/").
// Set<String> wholeHosts(List<Domain> domains) => {
//   for (final d in domains)
//     if (d.kind == 'app' && (d.pathPrefix.isEmpty || d.pathPrefix == '/'))
//       d.host,
// };
//
// /// Lists the paths of [app]'s hosts that other apps serve, with "Route a path to another app".
// class PathRoutesSection extends ConsumerWidget {
//   const PathRoutesSection({
//     super.key,
//     required this.app,
//     required this.domains,
//   });
//
//   final App app;
//   final List<Domain> domains;
//
//   @override
//   Widget build(BuildContext context, WidgetRef ref) {
//     final s = S.of(context);
//     final c = NectarColors.of(context);
//     final hosts = wholeHosts(domains);
//     if (hosts.isEmpty) return const SizedBox.shrink();
//     final canManage = ref.watch(
//       sessionProvider.select(
//         (x) => x.can('domain.manage', projectId: app.projectId),
//       ),
//     );
//     final rows = [
//       for (final r
//           in ref.watch(domainsOverviewProvider).value?.domains ??
//               const <DomainRow>[])
//         if (hosts.contains(r.domain.host) && r.appId != app.id) r,
//     ];
//     return Column(
//       crossAxisAlignment: CrossAxisAlignment.stretch,
//       children: [
//         const Divider(height: 22),
//         Row(
//           children: [
//             Expanded(
//               child: Text(
//                 s.pathRoutesTitle,
//                 style: const TextStyle(fontWeight: FontWeight.w600),
//               ),
//             ),
//             if (canManage)
//               TextButton.icon(
//                 onPressed: () =>
//                     showPathRouteDialog(context, app: app, domains: domains),
//                 icon: const Icon(Icons.alt_route_rounded, size: 18),
//                 label: Text(s.pathRoutesAdd),
//               ),
//           ],
//         ),
//         if (rows.isEmpty)
//           Text(
//             s.pathRoutesNone,
//             style: TextStyle(color: c.muted, fontSize: 12.5),
//           ),
//         for (final r in rows)
//           ListTile(
//             dense: true,
//             contentPadding: EdgeInsets.zero,
//             leading: const Icon(
//               Icons.subdirectory_arrow_right_rounded,
//               size: 18,
//             ),
//             title: Text(
//               '${r.domain.host}${r.domain.pathPrefix} → ${r.appName}'
//               '${r.domain.targetPort > 0 ? ':${r.domain.targetPort}' : ''}',
//               style: monoStyle(context, size: 13),
//             ),
//             subtitle: Text(
//               [
//                 r.domain.stripPrefix
//                     ? s.pathRoutesStripped(r.domain.pathPrefix)
//                     : s.pathRoutesKept,
//                 if (r.domain.status != 'active' &&
//                     r.domain.statusMessage.isNotEmpty)
//                   r.domain.statusMessage,
//               ].join(' · '),
//               style: TextStyle(color: c.muted, fontSize: 12),
//             ),
//             onTap: () => context.go('/apps/${r.appId}'),
//             trailing: canManage && r.canManage
//                 ? IconButton(
//                     tooltip: s.pathRoutesRemove,
//                     icon: const Icon(Icons.link_off_rounded, size: 18),
//                     onPressed: () => _remove(context, ref, r),
//                   )
//                 : null,
//           ),
//       ],
//     );
//   }
//
//   Future<void> _remove(BuildContext context, WidgetRef ref, DomainRow r) async {
//     final s = S.of(context);
//     if (!await confirmAction(
//       context,
//       s.pathRoutesRemoveTitle('${r.domain.host}${r.domain.pathPrefix}'),
//       s.pathRoutesRemoveBody(app.name),
//       s.pathRoutesRemove,
//       danger: true,
//     )) {
//       return;
//     }
//     try {
//       await ref
//           .read(apiProvider)
//           .delete('/apps/${r.appId}/domains/${r.domain.id}');
//       ref.invalidate(domainsOverviewProvider);
//       ref.invalidate(domainsProvider(r.appId));
//     } on Object catch (e) {
//       if (context.mounted) showSnack(context, describeError(e), error: true);
//     }
//   }
// }
//
// /// Routes host + path of [app] to another app of the project.
// Future<void> showPathRouteDialog(
//   BuildContext context, {
//   required App app,
//   required List<Domain> domains,
// }) => showDialog<void>(
//   context: context,
//   builder: (_) => _PathRouteDialog(app: app, domains: domains),
// );
//
// class _PathRouteDialog extends ConsumerStatefulWidget {
//   const _PathRouteDialog({required this.app, required this.domains});
//
//   final App app;
//   final List<Domain> domains;
//
//   @override
//   ConsumerState<_PathRouteDialog> createState() => _PathRouteDialogState();
// }
//
// class _PathRouteDialogState extends ConsumerState<_PathRouteDialog> {
//   late final List<Domain> _hosts = [
//     for (final d in widget.domains)
//       if (d.kind == 'app' && (d.pathPrefix.isEmpty || d.pathPrefix == '/')) d,
//   ];
//   late Domain _host = _hosts.first;
//   final _path = TextEditingController();
//   App? _target;
//   int _port = 0;
//   bool _strip = true;
//   bool _busy = false;
//
//   @override
//   void dispose() {
//     _path.dispose();
//     super.dispose();
//   }
//
//   List<int> _ports(App a) => [
//     a.port,
//     for (final p in (a.config['internal_ports'] as List? ?? const []))
//       if (p is num && p.toInt() != a.port) p.toInt(),
//   ];
//
//   String get _normPath {
//     var p = _path.text.trim();
//     if (p.isEmpty) return '';
//     if (!p.startsWith('/')) p = '/$p';
//     while (p.length > 1 && p.endsWith('/')) {
//       p = p.substring(0, p.length - 1);
//     }
//     return p;
//   }
//
//   Future<void> _save() async {
//     final s = S.of(context);
//     final target = _target!;
//     setState(() => _busy = true);
//     try {
//       await ref
//           .read(apiProvider)
//           .post(
//             '/apps/${target.id}/domains',
//             data: {
//               'host': _host.host,
//               'path_prefix': _normPath,
//               'target_port': _port == target.port ? 0 : _port,
//               'tls': _host.tls,
//               'force_https': _host.tls && _host.forceHttps,
//               'strip_prefix': _strip,
//             },
//           );
//       ref.invalidate(domainsOverviewProvider);
//       ref.invalidate(domainsProvider(target.id));
//       if (!mounted) return;
//       Navigator.pop(context);
//       showSnack(
//         context,
//         s.pathRoutesAdded('${_host.host}$_normPath', target.name),
//       );
//     } on Object catch (e) {
//       if (mounted) showSnack(context, describeError(e), error: true);
//     } finally {
//       if (mounted) setState(() => _busy = false);
//     }
//   }
//
//   @override
//   Widget build(BuildContext context) {
//     final s = S.of(context);
//     final c = NectarColors.of(context);
//     final candidates = [
//       for (final a
//           in ref.watch(projectAppsProvider(widget.app.projectId)).value ??
//               const <App>[])
//         if (a.id != widget.app.id &&
//             a.port > 0 &&
//             (a.kind == 'web' || a.kind == 'worker'))
//           a,
//     ];
//     final path = _normPath;
//     final validPath = path.length > 1 && !path.contains(RegExp(r'[\s?#*]'));
//     final example = path.isEmpty ? '/myCustomRoute' : path;
//     return AlertDialog(
//       title: Text(s.pathRoutesAdd),
//       content: SizedBox(
//         width: 480,
//         child: SingleChildScrollView(
//           child: Column(
//             mainAxisSize: MainAxisSize.min,
//             crossAxisAlignment: CrossAxisAlignment.stretch,
//             children: [
//               Text(
//                 s.pathRoutesHelp,
//                 style: TextStyle(color: c.muted, fontSize: 12.5),
//               ),
//               const SizedBox(height: 12),
//               DropdownButtonFormField<Domain>(
//                 initialValue: _host,
//                 decoration: InputDecoration(labelText: s.gatewayDomain),
//                 items: [
//                   for (final d in _hosts)
//                     DropdownMenuItem(value: d, child: Text(d.host)),
//                 ],
//                 onChanged: (v) => setState(() => _host = v ?? _host),
//               ),
//               const SizedBox(height: 12),
//               TextField(
//                 controller: _path,
//                 decoration: InputDecoration(
//                   labelText: s.domainPathPrefix,
//                   hintText: '/myCustomRoute',
//                 ),
//                 onChanged: (_) => setState(() {}),
//               ),
//               const SizedBox(height: 12),
//               if (candidates.isEmpty)
//                 Text(
//                   s.pathRoutesNoTargets,
//                   style: TextStyle(color: c.muted, fontSize: 12.5),
//                 )
//               else
//                 DropdownButtonFormField<App>(
//                   initialValue: _target,
//                   decoration: InputDecoration(labelText: s.pathRoutesTarget),
//                   items: [
//                     for (final a in candidates)
//                       DropdownMenuItem(
//                         value: a,
//                         child: Text('${a.name} (${a.kind}, :${a.port})'),
//                       ),
//                   ],
//                   onChanged: (v) => setState(() {
//                     _target = v;
//                     _port = v?.port ?? 0;
//                   }),
//                 ),
//               if (_target != null && _ports(_target!).length > 1) ...[
//                 const SizedBox(height: 12),
//                 DropdownButtonFormField<int>(
//                   initialValue: _port,
//                   decoration: InputDecoration(labelText: s.pathRoutesPort),
//                   items: [
//                     for (final p in _ports(_target!))
//                       DropdownMenuItem(value: p, child: Text('$p')),
//                   ],
//                   onChanged: (v) => setState(() => _port = v ?? _port),
//                 ),
//               ],
//               SwitchListTile(
//                 contentPadding: EdgeInsets.zero,
//                 value: _strip,
//                 onChanged: (v) => setState(() => _strip = v),
//                 title: Text(s.domainStripPrefix),
//                 subtitle: Text(
//                   _strip
//                       ? s.domainStripPrefixOn(example)
//                       : s.domainStripPrefixOff(example),
//                   style: TextStyle(color: c.muted, fontSize: 12.5),
//                 ),
//               ),
//             ],
//           ),
//         ),
//       ),
//       actions: [
//         TextButton(
//           onPressed: () => Navigator.pop(context),
//           child: Text(s.actionCancel),
//         ),
//         FilledButton(
//           onPressed: _busy || _target == null || !validPath ? null : _save,
//           child: Text(s.pathRoutesCreate),
//         ),
//       ],
//     );
//   }
// }

import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:nectar_apps_ui/theme/tokens.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../widgets/common.dart';

// Paths of an app's domains that go to other apps: app1.com/api → app2 (a web app or a worker with a
// port). Each is a domain of the other app with that host and path; the gateway takes the longest path.

/// Hosts an app serves as a whole (its own domains on "/").
Set<String> wholeHosts(List<Domain> domains) => {
  for (final d in domains)
    if (d.kind == 'app' && (d.pathPrefix.isEmpty || d.pathPrefix == '/'))
      d.host,
};

/// Lists the paths of [app]'s hosts that other apps serve, with "Route a path to another app".
class PathRoutesSection extends ConsumerWidget {
  const PathRoutesSection({
    super.key,
    required this.app,
    required this.domains,
  });

  final App app;
  final List<Domain> domains;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final hosts = wholeHosts(domains);
    if (hosts.isEmpty) return const SizedBox.shrink();
    final canManage = ref.watch(
      sessionProvider.select(
        (x) => x.can('domain.manage', projectId: app.projectId),
      ),
    );
    final rows = [
      for (final r
          in ref.watch(domainsOverviewProvider).value?.domains ??
              const <DomainRow>[])
        if (hosts.contains(r.domain.host) && r.appId != app.id) r,
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Divider(height: 22),
        Row(
          children: [
            Expanded(
              child: Text(
                s.pathRoutesTitle,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
            ),
            if (canManage)
              TextButton.icon(
                onPressed: () =>
                    showPathRouteDialog(context, app: app, domains: domains),
                icon: const Icon(Icons.alt_route_rounded, size: 18),
                label: Text(s.pathRoutesAdd),
              ),
          ],
        ),
        if (rows.isEmpty)
          Text(
            s.pathRoutesNone,
            style: TextStyle(color: c.muted, fontSize: 12.5),
          ),
        for (final r in rows)
          ListTile(
            dense: true,
            contentPadding: EdgeInsets.zero,
            leading: const Icon(
              Icons.subdirectory_arrow_right_rounded,
              size: 18,
            ),
            title: Text(
              '${r.domain.host}${r.domain.pathPrefix} → ${r.appName}'
              '${r.domain.targetPort > 0 ? ':${r.domain.targetPort}' : ''}',
              style: monoStyle(context, size: 13),
            ),
            subtitle: Text(
              [
                r.domain.stripPrefix
                    ? s.pathRoutesStripped(r.domain.pathPrefix)
                    : s.pathRoutesKept,
                if (r.domain.status != 'active' &&
                    r.domain.statusMessage.isNotEmpty)
                  r.domain.statusMessage,
              ].join(' · '),
              style: TextStyle(color: c.muted, fontSize: 12),
            ),
            onTap: () => context.go('/apps/${r.appId}'),
            trailing: canManage && r.canManage
                ? IconButton(
                    tooltip: s.pathRoutesRemove,
                    icon: const Icon(Icons.link_off_rounded, size: 18),
                    onPressed: () => _remove(context, ref, r),
                  )
                : null,
          ),
      ],
    );
  }

  Future<void> _remove(BuildContext context, WidgetRef ref, DomainRow r) async {
    final s = S.of(context);
    if (!await confirmAction(
      context,
      s.pathRoutesRemoveTitle('${r.domain.host}${r.domain.pathPrefix}'),
      s.pathRoutesRemoveBody(app.name),
      s.pathRoutesRemove,
      danger: true,
    )) {
      return;
    }
    try {
      await ref
          .read(apiProvider)
          .delete('/apps/${r.appId}/domains/${r.domain.id}');
      ref.invalidate(domainsOverviewProvider);
      ref.invalidate(domainsProvider(r.appId));
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }
}

/// Routes host + path of [app] to another app of the project.
Future<void> showPathRouteDialog(
  BuildContext context, {
  required App app,
  required List<Domain> domains,
}) => showDialog<void>(
  context: context,
  builder: (_) => _PathRouteDialog(app: app, domains: domains),
);

class _PathRouteDialog extends ConsumerStatefulWidget {
  const _PathRouteDialog({required this.app, required this.domains});

  final App app;
  final List<Domain> domains;

  @override
  ConsumerState<_PathRouteDialog> createState() => _PathRouteDialogState();
}

class _PathRouteDialogState extends ConsumerState<_PathRouteDialog> {
  late final List<Domain> _hosts = [
    for (final d in widget.domains)
      if (d.kind == 'app' && (d.pathPrefix.isEmpty || d.pathPrefix == '/')) d,
  ];
  late Domain _host = _hosts.first;
  final _path = TextEditingController();
  App? _target;
  int _port = 0;
  bool _strip = true;
  bool _busy = false;

  @override
  void dispose() {
    _path.dispose();
    super.dispose();
  }

  List<int> _ports(App a) => [
    a.port,
    for (final p in (a.config['internal_ports'] as List? ?? const []))
      if (p is num && p.toInt() != a.port) p.toInt(),
  ];

  String get _normPath {
    var p = _path.text.trim();
    if (p.isEmpty) return '';
    if (!p.startsWith('/')) p = '/$p';
    while (p.length > 1 && p.endsWith('/')) {
      p = p.substring(0, p.length - 1);
    }
    return p;
  }

  Future<void> _save() async {
    final s = S.of(context);
    final target = _target!;
    setState(() => _busy = true);
    try {
      await ref
          .read(apiProvider)
          .post(
            '/apps/${target.id}/domains',
            data: {
              'host': _host.host,
              'path_prefix': _normPath,
              'target_port': _port == target.port ? 0 : _port,
              'tls': _host.tls,
              'force_https': _host.tls && _host.forceHttps,
              'strip_prefix': _strip,
            },
          );
      ref.invalidate(domainsOverviewProvider);
      ref.invalidate(domainsProvider(target.id));
      if (!mounted) return;
      Navigator.pop(context);
      showSnack(
        context,
        s.pathRoutesAdded('${_host.host}$_normPath', target.name),
      );
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
    final candidates = [
      for (final a
          in ref.watch(projectAppsProvider(widget.app.projectId)).value ??
              const <App>[])
        if (a.id != widget.app.id &&
            a.port > 0 &&
            (a.kind == 'web' || a.kind == 'worker'))
          a,
    ];
    final path = _normPath;
    final validPath = path.length > 1 && !path.contains(RegExp(r'[\s?#*]'));
    final example = path.isEmpty ? '/myCustomRoute' : path;
    return AlertDialog(
      title: Text(s.pathRoutesAdd),
      content: SizedBox(
        width: 480,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                s.pathRoutesHelp,
                style: TextStyle(color: c.muted, fontSize: 12.5),
              ),
              const SizedBox(height: 12),
              DropdownButtonFormField<Domain>(
                initialValue: _host,
                decoration: InputDecoration(labelText: s.gatewayDomain),
                items: [
                  for (final d in _hosts)
                    DropdownMenuItem(value: d, child: Text(d.host)),
                ],
                onChanged: (v) => setState(() => _host = v ?? _host),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _path,
                decoration: InputDecoration(
                  labelText: s.domainPathPrefix,
                  hintText: '/myCustomRoute',
                ),
                onChanged: (_) => setState(() {}),
              ),
              const SizedBox(height: 12),
              if (candidates.isEmpty)
                Text(
                  s.pathRoutesNoTargets,
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                )
              else
                DropdownButtonFormField<App>(
                  initialValue: _target,
                  decoration: InputDecoration(labelText: s.pathRoutesTarget),
                  items: [
                    for (final a in candidates)
                      DropdownMenuItem(
                        value: a,
                        child: Text('${a.name} (${a.kind}, :${a.port})'),
                      ),
                  ],
                  onChanged: (v) => setState(() {
                    _target = v;
                    _port = v?.port ?? 0;
                  }),
                ),
              if (_target != null && _ports(_target!).length > 1) ...[
                const SizedBox(height: 12),
                DropdownButtonFormField<int>(
                  initialValue: _port,
                  decoration: InputDecoration(labelText: s.pathRoutesPort),
                  items: [
                    for (final p in _ports(_target!))
                      DropdownMenuItem(value: p, child: Text('$p')),
                  ],
                  onChanged: (v) => setState(() => _port = v ?? _port),
                ),
              ],
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: _strip,
                onChanged: (v) => setState(() => _strip = v),
                title: Text(s.domainStripPrefix),
                subtitle: Text(
                  _strip
                      ? s.domainStripPrefixOn(example)
                      : s.domainStripPrefixOff(example),
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: _busy || _target == null || !validPath ? null : _save,
          child: Text(s.pathRoutesCreate),
        ),
      ],
    );
  }
}
