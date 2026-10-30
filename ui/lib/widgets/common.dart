import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/client.dart';
import '../core/locale.dart';
import '../theme/tokens.dart';
import 'hex.dart';

/// Bordered surface card with optional header (title + trailing action).
class NCard extends StatelessWidget {
  const NCard({super.key, this.title, this.trailing, required this.child, this.padding, this.onTap});

  final String? title;
  final Widget? trailing;
  final Widget child;
  final EdgeInsetsGeometry? padding;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final body = Padding(
      padding: padding ?? const EdgeInsets.all(NectarTokens.gap + 2),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (title != null) ...[
            Row(
              children: [
                Expanded(child: Text(title!, style: Theme.of(context).textTheme.titleMedium)),
                ?trailing,
              ],
            ),
            const SizedBox(height: 14),
          ],
          child,
        ],
      ),
    );
    return Card(
      clipBehavior: Clip.antiAlias,
      child: onTap == null ? body : InkWell(onTap: onTap, child: body),
    );
  }
}

/// Standard page: title row, optional actions, scrollable content limited to a readable width.
class PageScaffold extends StatelessWidget {
  const PageScaffold({
    super.key,
    required this.title,
    this.subtitle,
    this.actions = const [],
    required this.children,
    this.onRefresh,
    this.maxWidth = 1280,
    this.leading,
  });

  final String title;
  final String? subtitle;
  final List<Widget> actions;
  final List<Widget> children;
  final Future<void> Function()? onRefresh;
  final double maxWidth;
  final Widget? leading;

  @override
  Widget build(BuildContext context) {
    final wide = MediaQuery.sizeOf(context).width >= 720;
    final pad = wide ? 32.0 : 16.0;
    final theme = Theme.of(context);
    Widget list = ListView(
      padding: EdgeInsets.fromLTRB(pad, wide ? 28 : 16, pad, 32),
      children: [
        Center(
          child: ConstrainedBox(
            constraints: BoxConstraints(maxWidth: maxWidth),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(
                  crossAxisAlignment: CrossAxisAlignment.center,
                  children: [
                    if (leading != null) ...[leading!, const SizedBox(width: 8)],
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(title, style: wide ? theme.textTheme.headlineMedium : theme.textTheme.headlineSmall),
                          if (subtitle != null) ...[
                            const SizedBox(height: 4),
                            Text(subtitle!, style: theme.textTheme.bodyMedium?.copyWith(color: NectarColors.of(context).muted)),
                          ],
                        ],
                      ),
                    ),
                    if (actions.isNotEmpty) Wrap(spacing: 8, runSpacing: 8, children: actions),
                  ],
                ),
                const SizedBox(height: NectarTokens.gapLarge),
                ...children,
              ],
            ),
          ),
        ),
      ],
    );
    if (onRefresh != null) {
      list = RefreshIndicator(onRefresh: onRefresh!, color: NectarTokens.amber, child: list);
    }
    return list;
  }
}

/// Small caps section label.
class SectionLabel extends StatelessWidget {
  const SectionLabel(this.text, {super.key, this.trailing});

  final String text;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.only(bottom: 12, top: 4),
        child: Row(
          children: [
            Expanded(child: Text(text.toUpperCase(), style: Theme.of(context).textTheme.labelSmall)),
            ?trailing,
          ],
        ),
      );
}

/// Centered loader for a section.
class LoadingBlock extends StatelessWidget {
  const LoadingBlock({super.key, this.height = 160});

  final double height;

  @override
  Widget build(BuildContext context) => SizedBox(height: height, child: const Center(child: HexLoader(size: 34)));
}

/// Error with a retry button. Shows the API message when there is one.
class ErrorBlock extends StatelessWidget {
  const ErrorBlock({super.key, required this.error, this.onRetry, this.compact = false});

  final Object error;
  final VoidCallback? onRetry;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Padding(
      padding: EdgeInsets.symmetric(vertical: compact ? 8 : 28),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.error_outline_rounded, color: c.failed, size: compact ? 20 : 28),
          const SizedBox(height: 8),
          Text(_headline(error), textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyMedium),
          if (_detail(error) case final detail?) ...[
            const SizedBox(height: 4),
            ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 520),
              child: Text(detail, textAlign: TextAlign.center, style: TextStyle(color: c.muted, fontSize: 12)),
            ),
          ],
          if (onRetry != null) ...[
            const SizedBox(height: 12),
            OutlinedButton.icon(onPressed: onRetry, icon: const Icon(Icons.refresh_rounded, size: 18), label: const Text('Retry')),
          ],
        ],
      ),
    );
  }
}

/// Short message for an error; technical details go to [_detail].
String _headline(Object error) {
  final msg = describeError(error);
  if (error is ApiException && error.status == 503 && msg.startsWith('kubernetes API unavailable')) {
    return 'The panel cannot reach Kubernetes.';
  }
  return msg;
}

String? _detail(Object error) {
  final msg = describeError(error);
  if (error is ApiException && error.status == 503 && msg.startsWith('kubernetes API unavailable')) {
    return msg.contains('127.0.0.1:8001')
        ? 'Development: start the dev cluster and kubectl proxy with `make dev-cluster` (or use `make dev`).'
        : msg;
  }
  return null;
}

/// Friendly empty state with a hex glyph, text and an optional action.
class EmptyState extends StatelessWidget {
  const EmptyState({super.key, required this.icon, required this.title, this.message, this.action});

  final IconData icon;
  final String title;
  final String? message;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 36, horizontal: 16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          HexBadge(size: 56, color: c.elevated, child: Icon(icon, color: NectarTokens.amber, size: 24)),
          const SizedBox(height: 14),
          Text(title, style: theme.textTheme.titleMedium, textAlign: TextAlign.center),
          if (message != null) ...[
            const SizedBox(height: 6),
            ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Text(message!, style: theme.textTheme.bodyMedium?.copyWith(color: c.muted), textAlign: TextAlign.center),
            ),
          ],
          if (action != null) ...[const SizedBox(height: 16), action!],
        ],
      ),
    );
  }
}

/// Renders an [AsyncValue] with the standard loading / error blocks.
class AsyncBlock<T> extends StatelessWidget {
  const AsyncBlock({super.key, required this.value, required this.data, this.onRetry, this.loadingHeight = 160});

  final AsyncValue<T> value;
  final Widget Function(T data) data;
  final VoidCallback? onRetry;
  final double loadingHeight;

  @override
  Widget build(BuildContext context) => value.when(
        // keep showing the previous data while refreshing
        skipLoadingOnRefresh: true,
        skipLoadingOnReload: true,
        data: data,
        loading: () => LoadingBlock(height: loadingHeight),
        error: (e, _) => ErrorBlock(error: e, onRetry: onRetry),
      );
}

void showSnack(BuildContext context, String message, {bool error = false}) {
  final c = NectarColors.of(context);
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(
      SnackBar(
        content: Row(
          children: [
            Icon(error ? Icons.error_outline_rounded : Icons.check_circle_outline_rounded,
                color: error ? c.failed : c.running, size: 18),
            const SizedBox(width: 10),
            Expanded(child: Text(message)),
          ],
        ),
      ),
    );
}

/// Underlined tab strip (scrolls horizontally on narrow screens).
class NTabs extends StatelessWidget {
  const NTabs({super.key, required this.index, required this.labels, required this.onChanged});

  final int index;
  final List<String> labels;
  final ValueChanged<int> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    return Container(
      decoration: BoxDecoration(border: Border(bottom: BorderSide(color: c.border))),
      child: SingleChildScrollView(
        scrollDirection: Axis.horizontal,
        child: Row(
        children: [
          for (var i = 0; i < labels.length; i++)
            InkWell(
              onTap: () => onChanged(i),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                decoration: BoxDecoration(
                  border: Border(bottom: BorderSide(color: i == index ? NectarTokens.amber : Colors.transparent, width: 2)),
                ),
                child: Text(
                  labels[i],
                  style: TextStyle(fontSize: 14, fontWeight: FontWeight.w600, color: i == index ? c.text : c.muted),
                ),
              ),
            ),
        ],
        ),
      ),
    );
  }
}

/// Asks for confirmation; true when the user chose [action].
Future<bool> confirmAction(BuildContext context, String title, String message, String action, {bool danger = false}) async =>
    await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(title),
        content: SizedBox(width: 420, child: Text(message)),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: Text(ctx.l10n.actionCancel)),
          FilledButton(
            style: danger ? FilledButton.styleFrom(backgroundColor: NectarColors.of(ctx).failed, foregroundColor: Colors.white) : null,
            onPressed: () => Navigator.pop(ctx, true),
            child: Text(action),
          ),
        ],
      ),
    ) ==
    true;

/// Asks for one line of text; null when cancelled.
Future<String?> promptText(BuildContext context, String title, String label, {String initial = ''}) =>
    showDialog<String>(context: context, builder: (_) => _PromptDialog(title: title, label: label, initial: initial));

class _PromptDialog extends StatefulWidget {
  const _PromptDialog({required this.title, required this.label, required this.initial});

  final String title;
  final String label;
  final String initial;

  @override
  State<_PromptDialog> createState() => _PromptDialogState();
}

class _PromptDialogState extends State<_PromptDialog> {
  late final _ctl = TextEditingController(text: widget.initial);

  @override
  void dispose() {
    _ctl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
        title: Text(widget.title),
        content: SizedBox(
          width: 420,
          child: TextField(
            controller: _ctl,
            autofocus: true,
            decoration: InputDecoration(labelText: widget.label),
            onSubmitted: (v) => Navigator.pop(context, v),
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, _ctl.text), child: const Text('OK')),
        ],
      );
}
