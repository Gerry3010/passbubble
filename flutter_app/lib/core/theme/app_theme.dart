// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';

/// Passbubble design system — phosphor terminal: green accents on near-black,
/// JetBrains Mono, minimal rounding. Tokens are the canonical brand palette
/// (extension/src/shared/theme.ts). See docs/design-guidelines.md.
///
/// Full-screen rule: body text stays light-gray ([onBg]); [green] is the accent,
/// not the body color (the extension/CLI render all-green; large screens don't).
class AppTheme {
  static const Color bg = Color(0xFF0A0A0A);
  static const Color surface = Color(0xFF0E140F);
  static const Color surfaceVariant = Color(0xFF121A14);
  static const Color green = Color(0xFF00FF41);
  static const Color greenDim = Color(0xFF19A23A);
  static const Color greenFaint = Color(0xFF122B18);
  static const Color onBg = Color(0xFFE0E0E0);
  static const Color onBgDim = Color(0xFF5F8C6A);
  static const Color error = Color(0xFFFF5F56);
  static const Color amber = Color(0xFFFFB000);
  static const Color border = Color(0xFF1D3A24);

  /// Subtle brand rounding: 2px for inputs/buttons, 4px for cards/surfaces.
  static const BorderRadius radiusSm = BorderRadius.all(Radius.circular(2));
  static const BorderRadius radiusMd = BorderRadius.all(Radius.circular(4));

  /// JetBrains Mono — the canonical brand font, with a disambiguated `0`/`O`.
  /// Use this (never the generic `fontFamily: 'monospace'`) wherever a user
  /// reads or copies exact characters: passwords, tokens, recovery codes,
  /// share links, command snippets. See docs/design-guidelines.md (§2).
  static TextStyle mono({
    double? fontSize,
    Color? color,
    FontWeight? fontWeight,
    double? letterSpacing,
    double? height,
    Color? backgroundColor,
    TextStyle? textStyle,
  }) =>
      GoogleFonts.jetBrainsMono(
        textStyle: textStyle,
        fontSize: fontSize,
        color: color,
        fontWeight: fontWeight,
        letterSpacing: letterSpacing,
        height: height,
        backgroundColor: backgroundColor,
      );

  static ThemeData get dark {
    final base = ThemeData.dark(useMaterial3: true);
    return base.copyWith(
      scaffoldBackgroundColor: bg,
      colorScheme: const ColorScheme.dark(
        brightness: Brightness.dark,
        primary: green,
        onPrimary: bg,
        secondary: greenDim,
        onSecondary: bg,
        surface: surface,
        onSurface: onBg,
        surfaceContainerHighest: surfaceVariant,
        error: error,
        outline: border,
      ),
      textTheme: GoogleFonts.jetBrainsMonoTextTheme(base.textTheme).apply(
        bodyColor: onBg,
        displayColor: green,
      ),
      appBarTheme: AppBarTheme(
        backgroundColor: bg,
        foregroundColor: green,
        elevation: 0,
        centerTitle: false,
        titleTextStyle: GoogleFonts.jetBrainsMono(
          fontSize: 16,
          fontWeight: FontWeight.w600,
          color: green,
          letterSpacing: 1.2,
        ),
      ),
      cardTheme: CardThemeData(
        color: surface,
        elevation: 0,
        shape: const RoundedRectangleBorder(
          borderRadius: radiusMd,
          side: BorderSide(color: border),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: surfaceVariant,
        border: const OutlineInputBorder(
          borderRadius: radiusSm,
          borderSide: BorderSide(color: border),
        ),
        enabledBorder: const OutlineInputBorder(
          borderRadius: radiusSm,
          borderSide: BorderSide(color: border),
        ),
        focusedBorder: const OutlineInputBorder(
          borderRadius: radiusSm,
          borderSide: BorderSide(color: green, width: 1.5),
        ),
        labelStyle: const TextStyle(color: onBgDim),
        hintStyle: const TextStyle(color: onBgDim),
        prefixIconColor: onBgDim,
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: green,
          foregroundColor: bg,
          shape: const RoundedRectangleBorder(borderRadius: radiusSm),
          textStyle: GoogleFonts.jetBrainsMono(
            fontWeight: FontWeight.w600,
            letterSpacing: 1.0,
          ),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: green,
          shape: const RoundedRectangleBorder(borderRadius: radiusSm),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: green,
          side: const BorderSide(color: green),
          shape: const RoundedRectangleBorder(borderRadius: radiusSm),
        ),
      ),
      listTileTheme: const ListTileThemeData(
        tileColor: surface,
        textColor: onBg,
        iconColor: onBgDim,
        shape: Border(bottom: BorderSide(color: border)),
      ),
      dividerTheme: const DividerThemeData(color: border, thickness: 1),
      floatingActionButtonTheme: const FloatingActionButtonThemeData(
        backgroundColor: green,
        foregroundColor: bg,
        shape: RoundedRectangleBorder(borderRadius: radiusSm),
      ),
      chipTheme: ChipThemeData(
        backgroundColor: surfaceVariant,
        labelStyle: const TextStyle(color: onBg),
        side: const BorderSide(color: border),
        shape: const RoundedRectangleBorder(borderRadius: radiusSm),
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: surfaceVariant,
        contentTextStyle: GoogleFonts.jetBrainsMono(color: onBg),
        shape: const RoundedRectangleBorder(borderRadius: radiusSm),
        behavior: SnackBarBehavior.floating,
      ),
      dialogTheme: const DialogThemeData(
        backgroundColor: surface,
        shape: RoundedRectangleBorder(borderRadius: radiusMd),
      ),
      bottomSheetTheme: const BottomSheetThemeData(
        backgroundColor: surface,
        shape: RoundedRectangleBorder(borderRadius: radiusMd),
      ),
      navigationRailTheme: const NavigationRailThemeData(
        backgroundColor: bg,
        selectedIconTheme: IconThemeData(color: green),
        unselectedIconTheme: IconThemeData(color: onBgDim),
        selectedLabelTextStyle: TextStyle(color: green),
        unselectedLabelTextStyle: TextStyle(color: onBgDim),
        indicatorColor: greenFaint,
        useIndicator: true,
      ),
    );
  }
}
