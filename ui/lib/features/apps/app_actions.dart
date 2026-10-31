import 'package:dio/dio.dart';
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
import '../auth/auth_frame.dart';
import 'artifact_file.dart';

/// Upload a new version of a file-based app and deploy it. Returns the started release.
Future<Release?> showUploadAndDeploy(BuildContext context, App app) =>
    showDialog<Release>(
      context: context,
      barrierDismissible: false,
      builder: (_) => _UploadDeployDialog(app: app),
    );

/// Image apps: change the image tag and deploy. Returns the started release.
Future<Release?> showChangeImage(BuildContext context, App app) =>
    showDialog<Release>(
      context: context,
      builder: (_) => _ChangeImageDialog(app: app),
    );

/// Deploy the current configuration again (reuses the live artifact). Returns the release or null on error.
Future<Release?> redeploy(
  BuildContext context,
  WidgetRef ref,
  App app, {
  bool force = false,
}) async {
  try {
    final res = await ref
        .read(apiProvider)
        .post('/apps/${app.id}/deploy', data: {if (force) 'force': true});
    return Release.fromJson(res);
  } on ApiException catch (e) {
    if (!context.mounted) return null;
    if (e.status == 409 && !e.message.contains('already in progress')) {
      final again = await _confirm(
        context,
        'Deploy anyway?',
        '${e.message}\n\nDeploying anyway may leave the app pending until resources free up.',
        'Deploy anyway',
      );
      if (again && context.mounted) {
        return redeploy(context, ref, app, force: true);
      }
      return null;
    }
    _snack(context, e.message, error: true);
    return null;
  }
}

/// Runs a live operation without a new release: scale (with data {'replicas': n}), start, stop or restart.
/// Asks before overcommitting the server. Returns true when it worked.
Future<bool> appOperation(
  BuildContext context,
  WidgetRef ref,
  App app,
  String op, {
  Map<String, dynamic>? data,
  String? done,
}) async {
  try {
    await ref
        .read(apiProvider)
        .post('/apps/${app.id}/$op', data: data ?? <String, dynamic>{});
  } on ApiException catch (e) {
    if (!context.mounted) return false;
    final capacity =
        e.status == 409 &&
        !e.message.contains('in progress') &&
        !e.message.contains('not deployed');
    if (capacity) {
      final again = await _confirm(
        context,
        'Not enough free resources',
        '${e.message}\n\nContinue anyway? New pods may stay pending until resources free up.',
        'Continue',
      );
      if (again && context.mounted) {
        return appOperation(
          context,
          ref,
          app,
          op,
          data: {...?data, 'force': true},
          done: done,
        );
      }
      return false;
    }
    _snack(context, e.message, error: true);
    return false;
  }
  refreshApp(ref, app.id);
  if (done != null && context.mounted) _snack(context, done);
  return true;
}

Future<Release?> rollbackTo(
  BuildContext context,
  WidgetRef ref,
  App app,
  Release target,
) async {
  final ok = await _confirm(
    context,
    'Roll back to release #${target.number}?',
    'Deploys the file, image and settings of release #${target.number} again as a new release.',
    'Roll back',
  );
  if (!ok || !context.mounted) return null;
  try {
    return Release.fromJson(
      await ref
          .read(apiProvider)
          .post('/apps/${app.id}/releases/${target.number}/rollback'),
    );
  } on ApiException catch (e) {
    if (context.mounted) _snack(context, e.message, error: true);
    return null;
  }
}

/// Deletes the app after the user types its slug. Returns true when deleted.
Future<bool> deleteApp(BuildContext context, WidgetRef ref, App app) async {
  final s = S.of(context);
  final ctl = TextEditingController();
  final ok = await showDialog<bool>(
    context: context,
    builder: (ctx) => StatefulBuilder(
      builder: (ctx, setState) => AlertDialog(
        title: Text(s.appDelete(app.name)),
        content: SizedBox(
          width: 420,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(s.appThisRemovesTheAppIts),
              const SizedBox(height: 14),
              Text(
                s.appTypeToConfirm(app.slug),
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 8),
              TextField(
                controller: ctl,
                autofocus: true,
                style: monoStyle(ctx, size: 14),
                onChanged: (_) => setState(() {}),
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
            style: FilledButton.styleFrom(
              backgroundColor: NectarColors.of(ctx).failed,
              foregroundColor: Colors.white,
            ),
            onPressed: ctl.text.trim() == app.slug
                ? () => Navigator.pop(ctx, true)
                : null,
            child: Text(s.actionDelete),
          ),
        ],
      ),
    ),
  );
  ctl.dispose();
  if (ok != true || !context.mounted) return false;
  try {
    await ref.read(apiProvider).delete('/apps/${app.id}');
    ref.invalidate(projectAppsProvider(app.projectId));
    ref.invalidate(projectsProvider);
    return true;
  } on ApiException catch (e) {
    if (context.mounted) _snack(context, e.message, error: true);
    return false;
  }
}

Future<bool> _confirm(
  BuildContext context,
  String title,
  String message,
  String action,
) async =>
    await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(title),
        content: SizedBox(width: 420, child: Text(message)),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(S.of(context).actionCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: Text(action),
          ),
        ],
      ),
    ) ==
    true;

void _snack(BuildContext context, String message, {bool error = false}) {
  final c = NectarColors.of(context);
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(
      content: Row(
        children: [
          Icon(
            error
                ? Icons.error_outline_rounded
                : Icons.check_circle_outline_rounded,
            color: error ? c.failed : c.running,
            size: 18,
          ),
          const SizedBox(width: 10),
          Expanded(child: Text(message)),
        ],
      ),
    ),
  );
}

class _UploadDeployDialog extends ConsumerStatefulWidget {
  const _UploadDeployDialog({required this.app});

  final App app;

  @override
  ConsumerState<_UploadDeployDialog> createState() =>
      _UploadDeployDialogState();
}

class _UploadDeployDialogState extends ConsumerState<_UploadDeployDialog> {
  PickedArtifact? _file;
  double? _progress;
  String? _error;
  Artifact? _artifact;
  CancelToken? _cancel;
  bool _deploying = false;

  Future<Runtime?> _runtime() async {
    final list = await ref.read(runtimesProvider.future);
    return list.where((r) => r.id == widget.app.runtime).firstOrNull;
  }

  Future<void> _pick() async {
    final rt = await _runtime();
    if (rt == null) return;
    PickedArtifact? f;
    try {
      f = await pickArtifact(rt);
    } on Object catch (e) {
      setState(() => _error = describeError(e));
      return;
    }
    if (f == null) return;
    final problem = artifactProblem(rt, f.name);
    if (problem != null) {
      setState(() => _error = problem);
      return;
    }
    final cancel = CancelToken();
    setState(() {
      _file = f;
      _error = null;
      _progress = 0;
      _cancel = cancel;
    });
    try {
      final art = await uploadArtifact(
        ref.read(apiProvider),
        widget.app.id,
        f,
        cancel: cancel,
        onProgress: (p) {
          if (mounted) setState(() => _progress = p);
        },
      );
      if (mounted) {
        setState(() {
          _artifact = art;
          _progress = null;
        });
      }
    } on Object catch (e) {
      if (mounted) {
        setState(() {
          _progress = null;
          _error = describeError(e);
        });
      }
    }
  }

  Future<void> _deploy() async {
    setState(() {
      _deploying = true;
      _error = null;
    });
    try {
      final res = await ref
          .read(apiProvider)
          .post(
            '/apps/${widget.app.id}/deploy',
            data: {'artifact_id': _artifact!.id},
          );
      if (mounted) Navigator.pop(context, Release.fromJson(res));
    } on Object catch (e) {
      if (mounted) {
        setState(() {
          _deploying = false;
          _error = describeError(e);
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final art = _artifact;
    final progress = _progress;
    return AlertDialog(
      title: Text(s.appDeployANewVersion),
      content: SizedBox(
        width: 460,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (_error != null) FormError(_error!),
            OutlinedButton.icon(
              onPressed: progress != null || _deploying ? null : _pick,
              icon: const Icon(Icons.upload_file_rounded, size: 18),
              label: Text(
                _file == null ? 'Choose file' : 'Choose another file',
              ),
            ),
            if (_file != null) ...[
              const SizedBox(height: 14),
              Text(
                _file!.name,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 4),
              if (progress != null) ...[
                ClipRRect(
                  borderRadius: BorderRadius.circular(99),
                  child: LinearProgressIndicator(value: progress, minHeight: 6),
                ),
                const SizedBox(height: 4),
                Text(
                  s.appUploading((progress * 100).round()),
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
              ] else if (art != null) ...[
                Text(
                  s.appUploadedAndChecked(formatBytes(art.size)),
                  style: TextStyle(color: c.running, fontSize: 12.5),
                ),
                for (final w in art.detect.warnings)
                  Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Text(
                      '⚠ $w',
                      style: TextStyle(color: c.warning, fontSize: 12.5),
                    ),
                  ),
              ],
            ],
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _deploying
              ? null
              : () {
                  _cancel?.cancel();
                  Navigator.pop(context);
                },
          child: Text(s.actionCancel),
        ),
        SizedBox(
          width: 120,
          child: BusyButton(
            label: s.appDeploy,
            busy: _deploying,
            onPressed: art == null ? null : _deploy,
          ),
        ),
      ],
    );
  }
}

class _ChangeImageDialog extends ConsumerStatefulWidget {
  const _ChangeImageDialog({required this.app});

  final App app;

  @override
  ConsumerState<_ChangeImageDialog> createState() => _ChangeImageDialogState();
}

class _ChangeImageDialogState extends ConsumerState<_ChangeImageDialog> {
  late final _image = TextEditingController(text: widget.app.image);
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _image.dispose();
    super.dispose();
  }

  Future<void> _go() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    final api = ref.read(apiProvider);
    try {
      await api.patch(
        '/apps/${widget.app.id}',
        data: {
          'config': {'image': _image.text.trim()},
        },
      );
      final res = await api.post('/apps/${widget.app.id}/deploy');
      if (mounted) Navigator.pop(context, Release.fromJson(res));
    } on Object catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = describeError(e);
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return AlertDialog(
      title: Text(s.appDeployAnotherImage),
      content: SizedBox(
        width: 460,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (_error != null) FormError(_error!),
            TextField(
              controller: _image,
              autofocus: true,
              style: monoStyle(context, size: 14),
              decoration: InputDecoration(labelText: s.appImage),
              onSubmitted: (_) => _go(),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.pop(context),
          child: Text(s.actionCancel),
        ),
        SizedBox(
          width: 120,
          child: BusyButton(label: s.appDeploy, busy: _busy, onPressed: _go),
        ),
      ],
    );
  }
}
