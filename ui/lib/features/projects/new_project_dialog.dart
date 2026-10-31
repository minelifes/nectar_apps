import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../auth/auth_frame.dart';

/// "My Shop!" → "my-shop" (the API's slug rules: 1–30 of a-z, 0-9, dash; no dash at the ends).
String slugify(String name) {
  var s = name.toLowerCase().replaceAll(RegExp(r'[^a-z0-9]+'), '-');
  s = s.replaceAll(RegExp(r'^-+|-+$'), '');
  if (s.length > 30) s = s.substring(0, 30).replaceAll(RegExp(r'-+$'), '');
  return s;
}

final _slugRe = RegExp(r'^[a-z0-9]([a-z0-9-]{0,28}[a-z0-9])?$');

/// Creates a project and opens it. Quota presets come with the project settings later; new projects
/// are unlimited (the default).
Future<void> showNewProjectDialog(BuildContext context, WidgetRef ref) async {
  final created = await showDialog<Project>(
    context: context,
    builder: (_) => const NewProjectDialog(),
  );
  if (created != null && context.mounted) {
    ref.invalidate(projectsProvider);
    context.go('/projects/${created.id}');
  }
}

/// Dialog that creates a project and pops it (or null).
class NewProjectDialog extends ConsumerStatefulWidget {
  const NewProjectDialog({super.key});

  @override
  ConsumerState<NewProjectDialog> createState() => _NewProjectDialogState();
}

class _NewProjectDialogState extends ConsumerState<NewProjectDialog> {
  final _form = GlobalKey<FormState>();
  final _name = TextEditingController();
  final _slug = TextEditingController();
  bool _slugEdited = false;
  bool _busy = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _name.addListener(() {
      if (!_slugEdited) _slug.text = slugify(_name.text);
    });
  }

  @override
  void dispose() {
    _name.dispose();
    _slug.dispose();
    super.dispose();
  }

  Future<void> _create() async {
    if (!_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final res = await ref
          .read(apiProvider)
          .post(
            '/projects',
            data: {'name': _name.text.trim(), 'slug': _slug.text.trim()},
          );
      if (mounted) Navigator.of(context).pop(Project.fromJson(res));
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    return AlertDialog(
      title: Text(s.projectNewProject),
      content: SizedBox(
        width: 420,
        child: Form(
          key: _form,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (_error != null) FormError(_error!),
              TextFormField(
                controller: _name,
                autofocus: true,
                decoration: InputDecoration(
                  labelText: s.backupName,
                  hintText: s.projectMyShop,
                ),
                textInputAction: TextInputAction.next,
                validator: (v) =>
                    (v ?? '').trim().isEmpty ? 'Enter a name' : null,
              ),
              const SizedBox(height: 14),
              TextFormField(
                controller: _slug,
                style: monoStyle(context, size: 14),
                decoration: InputDecoration(labelText: s.projectSlug),
                onChanged: (_) => _slugEdited = true,
                onFieldSubmitted: (_) => _create(),
                validator: (v) => _slugRe.hasMatch((v ?? '').trim())
                    ? null
                    : '1–30 lowercase letters, digits or dashes',
              ),
              const SizedBox(height: 8),
              ValueListenableBuilder(
                valueListenable: _slug,
                builder: (context, value, _) => Text(
                  'Kubernetes namespace: prj-${value.text.isEmpty ? '…' : value.text}',
                  style: TextStyle(color: c.muted, fontSize: 12.5),
                ),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.of(context).pop(),
          child: Text(s.actionCancel),
        ),
        SizedBox(
          width: 120,
          child: BusyButton(
            label: s.projectCreate,
            busy: _busy,
            onPressed: _create,
          ),
        ),
      ],
    );
  }
}
