import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/copy.dart';

// An app marked as a database (an imported MySQL, a PostgreSQL image…): it stays an app and also gets what
// database add-ons have — `nectar db connect`, dump backups and connection variables for other apps.

/// Engines an app can be marked as, with their display names and usual ports.
const appDatabaseEngines = <String, (String, int)>{
  'postgres': ('PostgreSQL', 5432),
  'mysql': ('MySQL', 3306),
  'mariadb': ('MariaDB', 3306),
  'redis': ('Redis', 6379),
};

String appDatabaseEngineName(String engine) =>
    appDatabaseEngines[engine]?.$1 ?? engine;

/// The apps whose variables reference a database app.
final appDatabaseLinksProvider = FutureProvider.autoDispose
    .family<List<Map<String, dynamic>>, int>((ref, appId) async {
      final res = await ref
          .watch(apiProvider)
          .get('/apps/$appId/database/links');
      return [
        for (final l in (res as List)) Map<String, dynamic>.from(l as Map),
      ];
    });

/// Marks an app as a database, or changes / removes the marking.
Future<void> showAppDatabaseDialog(
  BuildContext context,
  WidgetRef ref,
  App app,
) => showDialog<void>(
  context: context,
  builder: (_) => _MarkDialog(app: app),
);

class _MarkDialog extends ConsumerStatefulWidget {
  const _MarkDialog({required this.app});

  final App app;

  @override
  ConsumerState<_MarkDialog> createState() => _MarkDialogState();
}

class _MarkDialogState extends ConsumerState<_MarkDialog> {
  List<String>? _vars;
  Object? _error;
  bool _busy = false;
  String _engine = 'postgres';
  String? _userVar, _passwordVar, _databaseVar;
  final _user = TextEditingController();
  final _database = TextEditingController();
  final _port = TextEditingController();

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _user.dispose();
    _database.dispose();
    _port.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final res =
          await ref
                  .read(apiProvider)
                  .get('/apps/${widget.app.id}/database/detect')
              as Map;
      final vars = [
        for (final v in (res['variables'] as List? ?? const [])) '$v',
      ];
      // the current marking wins over the suggestion
      final cur =
          widget.app.database ?? AppDatabaseMark.fromJson(res['suggestion']);
      if (!mounted) return;
      setState(() {
        _vars = vars;
        if (cur.engine.isNotEmpty) _engine = cur.engine;
        _userVar = vars.contains(cur.userVar) ? cur.userVar : null;
        _passwordVar = vars.contains(cur.passwordVar) ? cur.passwordVar : null;
        _databaseVar = vars.contains(cur.databaseVar) ? cur.databaseVar : null;
        _user.text = cur.user;
        _database.text = cur.database;
        _port.text = cur.port > 0 ? '${cur.port}' : '';
      });
    } on Object catch (e) {
      if (mounted) setState(() => _error = e);
    }
  }

  Future<void> _save() async {
    final s = S.of(context);
    setState(() => _busy = true);
    try {
      await ref
          .read(apiProvider)
          .put(
            '/apps/${widget.app.id}/database',
            data: {
              'engine': _engine,
              'port': int.tryParse(_port.text.trim()) ?? 0,
              'user': _userVar == null ? _user.text.trim() : '',
              'user_var': _userVar ?? '',
              'password_var': _passwordVar ?? '',
              'database': _databaseVar == null ? _database.text.trim() : '',
              'database_var': _databaseVar ?? '',
            },
          );
      refreshApp(ref, widget.app.id);
      ref.invalidate(projectAppsProvider(widget.app.projectId));
      if (!mounted) return;
      Navigator.pop(context);
      showSnack(context, s.appDbMarked(widget.app.name));
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _unmark() async {
    final s = S.of(context);
    if (!await confirmAction(
      context,
      s.appDbUnmarkTitle(widget.app.name),
      s.appDbUnmarkBody,
      s.appDbUnmark,
      danger: true,
    )) {
      return;
    }
    if (!mounted) return;
    try {
      await ref.read(apiProvider).delete('/apps/${widget.app.id}/database');
      refreshApp(ref, widget.app.id);
      ref.invalidate(projectAppsProvider(widget.app.projectId));
      if (mounted) Navigator.pop(context);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final vars = _vars;
    final redis = _engine == 'redis';
    Widget varPicker(
      String label,
      String? value,
      ValueChanged<String?> onChanged, {
      String? none,
    }) => DropdownButtonFormField<String?>(
      initialValue: value,
      isExpanded: true,
      decoration: InputDecoration(labelText: label),
      items: [
        DropdownMenuItem<String?>(
          value: null,
          child: Text(none ?? s.appDbNoVariable),
        ),
        for (final v in vars ?? const <String>[])
          DropdownMenuItem<String?>(
            value: v,
            child: Text(v, style: monoStyle(context)),
          ),
      ],
      onChanged: onChanged,
    );

    Widget content;
    if (_error != null) {
      content = Text(describeError(_error!), style: TextStyle(color: c.failed));
    } else if (vars == null) {
      content = const Padding(
        padding: EdgeInsets.all(24),
        child: Center(child: CircularProgressIndicator()),
      );
    } else {
      content = SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              s.appDbMarkHelp,
              style: TextStyle(color: c.muted, fontSize: 12.5),
            ),
            const SizedBox(height: NectarTokens.gap),
            DropdownButtonFormField<String>(
              initialValue: _engine,
              decoration: InputDecoration(labelText: s.appDbEngine),
              items: [
                for (final e in appDatabaseEngines.entries)
                  DropdownMenuItem(value: e.key, child: Text(e.value.$1)),
              ],
              onChanged: (v) => setState(() => _engine = v ?? _engine),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _port,
              keyboardType: TextInputType.number,
              decoration: InputDecoration(
                labelText: s.appDbPort,
                hintText: '${appDatabaseEngines[_engine]?.$2 ?? ''}',
              ),
            ),
            const SizedBox(height: 12),
            varPicker(
              redis ? s.appDbPasswordVarOptional : s.appDbPasswordVar,
              _passwordVar,
              (v) => setState(() => _passwordVar = v),
            ),
            if (!redis) ...[
              const SizedBox(height: 12),
              varPicker(
                s.appDbUserVar,
                _userVar,
                (v) => setState(() => _userVar = v),
                none: s.appDbTypeIt,
              ),
              if (_userVar == null)
                TextField(
                  controller: _user,
                  decoration: InputDecoration(
                    labelText: s.appDbUser,
                    hintText: _engine == 'postgres' ? 'postgres' : 'root',
                  ),
                ),
              const SizedBox(height: 12),
              varPicker(
                s.appDbDatabaseVar,
                _databaseVar,
                (v) => setState(() => _databaseVar = v),
                none: s.appDbTypeIt,
              ),
              if (_databaseVar == null)
                TextField(
                  controller: _database,
                  decoration: InputDecoration(labelText: s.appDbDatabase),
                ),
            ],
          ],
        ),
      );
    }
    return AlertDialog(
      title: Text(
        widget.app.database == null ? s.appDbMarkTitle : s.appDbSettingsTitle,
      ),
      content: SizedBox(width: 480, child: content),
      actions: [
        if (widget.app.database != null)
          TextButton(
            onPressed: _busy ? null : _unmark,
            child: Text(s.appDbUnmark),
          ),
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        FilledButton(
          onPressed: vars == null || _busy || (!redis && _passwordVar == null)
              ? null
              : _save,
          child: Text(s.actionSave),
        ),
      ],
    );
  }
}

/// Overview card of a database app: how to connect, credentials, linked apps.
class AppDatabaseCard extends ConsumerWidget {
  const AppDatabaseCard({super.key, required this.app});

  final App app;

  Future<void> _showCredentials(BuildContext context, WidgetRef ref) async {
    final s = S.of(context);
    try {
      final cr =
          await ref
                  .read(apiProvider)
                  .get('/apps/${app.id}/database/credentials')
              as Map;
      if (!context.mounted) return;
      final env = Map<String, dynamic>.from(cr['env'] as Map? ?? const {});
      final url = '${env['DATABASE_URL'] ?? env['REDIS_URL'] ?? ''}';
      await showDialog<void>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text(s.appDbCredentials),
          content: SizedBox(
            width: 520,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                for (final (k, v) in [
                  (s.appDbHost, '${cr['host']}:${cr['port']}'),
                  if ('${cr['user'] ?? ''}'.isNotEmpty)
                    (s.appDbUser, '${cr['user']}'),
                  if ('${cr['password'] ?? ''}'.isNotEmpty)
                    (s.appDbPassword, '${cr['password']}'),
                  if ('${cr['database'] ?? ''}'.isNotEmpty)
                    (s.appDbDatabase, '${cr['database']}'),
                  if (url.isNotEmpty) ('URL', url),
                ])
                  Padding(
                    padding: const EdgeInsets.symmetric(vertical: 4),
                    child: Row(
                      children: [
                        SizedBox(width: 90, child: Text(k)),
                        Expanded(
                          child: SelectableText(
                            v,
                            style: monoStyle(ctx, size: 12.5),
                          ),
                        ),
                        IconButton(
                          tooltip: s.actionCopy,
                          icon: const Icon(Icons.copy_rounded, size: 16),
                          onPressed: () => copyText(ctx, v),
                        ),
                      ],
                    ),
                  ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: Text(s.actionClose),
            ),
          ],
        ),
      );
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _link(
    BuildContext context,
    WidgetRef ref,
    List<App> candidates,
  ) async {
    final s = S.of(context);
    App? target = candidates.isEmpty ? null : candidates.first;
    final prefix = TextEditingController();
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setState) => AlertDialog(
          title: Text(s.appDbLinkTitle(app.name)),
          content: SizedBox(
            width: 440,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  s.appDbLinkHelp('\${${app.slug}.DATABASE_URL}'),
                  style: TextStyle(
                    color: NectarColors.of(ctx).muted,
                    fontSize: 12.5,
                  ),
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<App>(
                  initialValue: target,
                  decoration: InputDecoration(labelText: s.appDbLinkApp),
                  items: [
                    for (final a in candidates)
                      DropdownMenuItem(value: a, child: Text(a.name)),
                  ],
                  onChanged: (v) => setState(() => target = v),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: prefix,
                  decoration: InputDecoration(
                    labelText: s.appDbLinkPrefix,
                    hintText: 'e.g. ORDERS_',
                  ),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: Text(s.actionCancel),
            ),
            FilledButton(
              onPressed: target == null ? null : () => Navigator.pop(ctx, true),
              child: Text(s.appDbLink),
            ),
          ],
        ),
      ),
    );
    final p = prefix.text.trim().toUpperCase();
    prefix.dispose();
    if (ok != true || target == null || !context.mounted) return;
    try {
      await ref
          .read(apiProvider)
          .post(
            '/apps/${app.id}/database/links',
            data: {'app_id': target!.id, 'prefix': p},
          );
      ref.invalidate(appDatabaseLinksProvider(app.id));
      if (context.mounted) showSnack(context, s.appDbLinked(target!.name));
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _unlink(
    BuildContext context,
    WidgetRef ref,
    Map<String, dynamic> l,
  ) async {
    final s = S.of(context);
    if (!await confirmAction(
      context,
      s.appDbUnlinkTitle('${l['name']}'),
      s.appDbUnlinkBody(app.slug),
      s.appDbUnlink,
    )) {
      return;
    }
    try {
      await ref
          .read(apiProvider)
          .delete('/apps/${app.id}/database/links/${l['app_id']}');
      ref.invalidate(appDatabaseLinksProvider(app.id));
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final db = app.database!;
    final session = ref.watch(sessionProvider);
    final canSecrets = session.can(
      'env.secrets.view',
      projectId: app.projectId,
    );
    final canLink = session.can('env.edit', projectId: app.projectId);
    final project = (ref.watch(projectsProvider).value ?? const <Project>[])
        .where((p) => p.id == app.projectId)
        .firstOrNull;
    final port = db.port > 0
        ? db.port
        : (appDatabaseEngines[db.engine]?.$2 ?? app.port);
    final connect =
        'nectar db connect ${project?.slug ?? '<project>'}/${app.slug}';
    final links = ref.watch(appDatabaseLinksProvider(app.id));
    final others =
        (ref.watch(projectAppsProvider(app.projectId)).value ?? const <App>[])
            .where(
              (a) =>
                  a.id != app.id &&
                  (links.value ?? const []).every((l) => l['app_id'] != a.id),
            )
            .toList();
    final small = TextStyle(color: c.muted, fontSize: 12.5);
    return NCard(
      title: s.appDbCardTitle(appDatabaseEngineName(db.engine)),
      trailing: canSecrets
          ? TextButton(
              onPressed: () => _showCredentials(context, ref),
              child: Text(s.appDbShowCredentials),
            )
          : null,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(s.appDbInsideProject, style: small),
          SelectableText('${app.slug}:$port', style: monoStyle(context)),
          const SizedBox(height: 10),
          Text(s.appDbFromYourComputer, style: small),
          Row(
            children: [
              Expanded(
                child: SelectableText(
                  connect,
                  style: monoStyle(context, size: 12.5),
                ),
              ),
              IconButton(
                tooltip: s.actionCopy,
                icon: const Icon(Icons.copy_rounded, size: 16),
                onPressed: () => copyText(context, connect),
              ),
            ],
          ),
          const Divider(height: 22),
          Row(
            children: [
              Expanded(
                child: Text(
                  s.appDbLinkedApps,
                  style: const TextStyle(fontWeight: FontWeight.w600),
                ),
              ),
              if (canLink)
                TextButton.icon(
                  onPressed: others.isEmpty
                      ? null
                      : () => _link(context, ref, others),
                  icon: const Icon(Icons.link_rounded, size: 18),
                  label: Text(s.appDbLinkAction),
                ),
            ],
          ),
          AsyncBlock(
            value: links,
            loadingHeight: 40,
            onRetry: () => ref.invalidate(appDatabaseLinksProvider(app.id)),
            data: (list) => list.isEmpty
                ? Text(
                    s.appDbNoLinks('\${${app.slug}.DATABASE_URL}'),
                    style: small,
                  )
                : Column(
                    children: [
                      for (final l in list)
                        ListTile(
                          dense: true,
                          contentPadding: EdgeInsets.zero,
                          leading: const Icon(Icons.apps_rounded, size: 18),
                          title: Text('${l['name']}'),
                          subtitle: Text(
                            '${l['prefix']}'.isEmpty
                                ? s.appDbNoPrefix
                                : s.appDbPrefix('${l['prefix']}'),
                            style: small,
                          ),
                          trailing: canLink
                              ? IconButton(
                                  tooltip: s.appDbUnlink,
                                  icon: const Icon(
                                    Icons.link_off_rounded,
                                    size: 18,
                                  ),
                                  onPressed: () => _unlink(context, ref, l),
                                )
                              : null,
                        ),
                    ],
                  ),
          ),
          const SizedBox(height: 8),
          Text(s.appDbBackupsNote, style: small),
        ],
      ),
    );
  }
}
