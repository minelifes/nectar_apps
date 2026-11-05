import 'package:flutter/material.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final xmlSyntaxRules = <SyntaxRule>[
  // XML comments
  SyntaxRule(
    RegExp(r'<!--[\s\S]*?-->'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // CDATA
  SyntaxRule(
    RegExp(r'<!\[CDATA\[[\s\S]*?\]\]>'),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Processing instructions
  SyntaxRule(
    RegExp(r'<\?[\s\S]*?\?>'),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // DOCTYPE and declarations
  SyntaxRule(
    RegExp(r'<!DOCTYPE\b[^>]*>|<![A-Z][^>]*>'),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // Opening and closing tag names
  SyntaxRule(
    RegExp(r'</?[a-zA-Z_][\w:.-]*'),
    const TextStyle(color: Color(0xFFE06C75), fontWeight: FontWeight.bold),
  ),

  // Attribute names
  SyntaxRule(
    RegExp(r'(?<=[\s<])[a-zA-Z_:][\w:.-]*(?=\s*=)'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Quoted attribute values
  SyntaxRule(
    RegExp(r'''"[^"]*"|'[^']*' '''),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Entities
  SyntaxRule(
    RegExp(r'&(?:#\d+|#x[0-9a-fA-F]+|[a-zA-Z]\w+);'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Tag delimiters
  SyntaxRule(RegExp(r'/?>'), const TextStyle(color: Color(0xFFE06C75))),

  // Equals
  SyntaxRule(RegExp(r'='), const TextStyle(color: Color(0xFFABB2BF))),
];
