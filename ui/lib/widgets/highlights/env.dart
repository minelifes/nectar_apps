import 'package:flutter/material.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final envSyntaxRules = <SyntaxRule>[
  // Comments
  SyntaxRule(
    RegExp(r'^\s*#[^\n]*'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // export keyword
  SyntaxRule(
    RegExp(r'\bexport\b'),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // Variable names
  SyntaxRule(
    RegExp(r'^[ \t]*(?:export[ \t]+)?[a-zA-Z_][a-zA-Z0-9_]*(?=\s*=)'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // Double-quoted values
  SyntaxRule(
    RegExp(r'"(?:[^"\\]|\\.)*"'),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Single-quoted values
  SyntaxRule(RegExp(r"'[^']*'"), const TextStyle(color: Color(0xFF98C379))),

  // Variable interpolation
  SyntaxRule(
    RegExp(r'\$\{[a-zA-Z_][a-zA-Z0-9_]*[^}]*\}|\$[a-zA-Z_][a-zA-Z0-9_]*'),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // Special variables
  SyntaxRule(
    RegExp(r'\$\{(?:HOME|USER|PATH|PWD|SHELL|HOSTNAME)\}'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Boolean values
  SyntaxRule(
    RegExp(r'\b(true|false|yes|no|on|off)\b', caseSensitive: false),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // Numbers
  SyntaxRule(
    RegExp(r'\b\d+(?:\.\d+)?\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Assignment
  SyntaxRule(RegExp(r'='), const TextStyle(color: Color(0xFFE06C75))),
];
