#include <assert.h>
#include "../menu_darwin.m"

void stmMenuAction(int action) {}
void stmTunnelAction(const char *name, int enabled) {}
void stmConnectionsAction(const char *name, int history) {}

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        NSArray *lines = @[
            @{@"name": @"ssh", @"rateText": @"↓ 0 B/s  ↑ 0 B/s"},
            @{@"name": @"PostgreSQL production", @"rateText": @"↓ 1.0 KiB/s  ↑ 512 B/s"},
            @{@"name": @"A very long service name that must be truncated", @"rateText": @"↓ 1023.9 MiB/s  ↑ 999.9 MiB/s"},
            @{@"name": @"测试线路 👨‍👩‍👧‍👦", @"rateText": @"速率采样中"},
        ];
        CGFloat width = stmLineTitleWidth(lines);
        assert(stmLineTitleWidth(@[@{@"name": @"ssh", @"rateText": @"↓ 0 B/s  ↑ 0 B/s"}]) == 240);
        NSBitmapImageRep *preview = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:(width + 24) * 2 pixelsHigh:224
            bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
        preview.size = NSMakeSize(width + 24, 112);
        [NSGraphicsContext saveGraphicsState];
        NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:preview];
        [NSColor.whiteColor setFill];
        NSRectFill(NSMakeRect(0, 0, width + 24, 112));
        NSInteger row = 0;
        for (NSDictionary *line in lines) {
            NSAttributedString *title = stmLineTitle(line[@"name"], line[@"rateText"], width);
            assert(fabs(title.size.width - width) <= 1);
            NSRange separator = [title.string rangeOfString:@"\t"];
            NSAttributedString *name = [title attributedSubstringFromRange:NSMakeRange(0, separator.location)];
            assert(name.size.width <= stmLineNameWidth + 1);
            NSTextStorage *storage = [[NSTextStorage alloc] initWithAttributedString:title];
            NSLayoutManager *layout = [NSLayoutManager new];
            NSTextContainer *container = [[NSTextContainer alloc] initWithSize:NSMakeSize(width + 1, 30)];
            container.lineFragmentPadding = 0;
            [layout addTextContainer:container];
            [storage addLayoutManager:layout];
            NSRange textRange = [title.string rangeOfString:line[@"rateText"] options:NSBackwardsSearch];
            NSRange glyphRange = [layout glyphRangeForCharacterRange:textRange actualCharacterRange:NULL];
            NSRect bounds = [layout boundingRectForGlyphRange:glyphRange inTextContainer:container];
            if (fabs(NSMaxX(bounds) - width) > 1) {
                fprintf(stderr, "Rate right edge %.2f, expected %.2f\n", NSMaxX(bounds), width);
                return 1;
            }
            assert(NSMaxY(bounds) < 30);
            [title drawInRect:NSMakeRect(12, 82 - row * 24, width + 1, 24)];
            row++;
        }
        [NSGraphicsContext restoreGraphicsState];
        if (argc > 1) assert([[preview representationUsingType:NSBitmapImageFileTypePNG properties:@{}] writeToFile:[NSString stringWithUTF8String:argv[1]] atomically:YES]);
        puts("Line rates: common right edge, long names truncated, single-row layout.");
    }
    return 0;
}
