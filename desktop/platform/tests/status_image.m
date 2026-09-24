// Headless regression check: clang -fobjc-arc -framework Cocoa -framework CoreText
// status_image.m -o /tmp/portway-status-image-test && /tmp/portway-status-image-test
#include <assert.h>
#include "../menu_darwin.m"

void stmMenuAction(int action) {}
void stmTunnelAction(const char *name, int enabled) {}
void stmConnectionsAction(const char *name, int history) {}

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        NSImage *symbol = [NSImage imageWithSystemSymbolName:@"arrow.left.arrow.right" accessibilityDescription:nil];
        symbol.size = NSMakeSize(18, 18);
        NSArray *samples = @[@"--\n--", @"0 B/s\n0 B/s", @"512 B/s\n1.0 KiB/s", @"999.9 MiB/s\n1023.9 KiB/s"];
        for (NSString *sample in samples) {
            NSImage *image = stmStatusImage(sample, symbol);
            assert(image.template);
            assert(image.size.height == 20 && image.size.width > 22);
            if ([sample isEqualToString:@"0 B/s\n0 B/s"]) assert(image.size.width < 50);
            NSInteger width = (NSInteger)image.size.width * 2;
            NSBitmapImageRep *bitmap = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:width pixelsHigh:40
                bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
            bitmap.size = image.size;
            [NSGraphicsContext saveGraphicsState];
            NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:bitmap];
            CGContextClearRect(NSGraphicsContext.currentContext.CGContext, CGRectMake(0, 0, width, 40));
            [image drawInRect:NSMakeRect(0, 0, image.size.width, image.size.height)];
            [NSGraphicsContext restoreGraphicsState];
            if (argc > 1) {
                NSBitmapImageRep *preview = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:width pixelsHigh:40
                    bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
                [NSGraphicsContext saveGraphicsState];
                NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:preview];
                [NSColor.whiteColor setFill];
                NSRectFill(NSMakeRect(0, 0, width, 40));
                [bitmap drawInRect:NSMakeRect(0, 0, width, 40) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:NO hints:nil];
                [NSGraphicsContext restoreGraphicsState];
                NSData *png = [preview representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
                assert([png writeToFile:[NSString stringWithUTF8String:argv[1]] atomically:YES]);
            }
            // Both rows must have ink, with padding above/below and between them.
            for (NSInteger row = 0; row < 2; row++) {
                NSInteger minimum = 40, maximum = -1;
                for (NSInteger y = row * 20; y < (row + 1) * 20; y++) {
                    for (NSInteger x = 42; x < width; x++) {
                        if ([[bitmap colorAtX:x y:y] alphaComponent] > 0.1) {
                            minimum = MIN(minimum, y);
                            maximum = MAX(maximum, y);
                        }
                    }
                }
                assert(maximum >= minimum);
                if (minimum <= row * 20 || maximum >= (row + 1) * 20 - 1) {
                    fprintf(stderr, "%s row %ld: ink y=%ld..%ld\n", sample.UTF8String, (long)row, (long)minimum, (long)maximum);
                    return 1;
                }
            }
        }
        puts("Status image: both rows fit inside 20pt with vertical padding.");
    }
    return 0;
}
