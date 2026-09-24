#include <assert.h>
#include "../menu_darwin.m"

void stmMenuAction(int action) {}
void stmTunnelAction(const char *name, int enabled) {}
void stmConnectionsAction(const char *name, int history) {}

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        stateImages = [NSMutableDictionary new];
        NSArray *services = @[@"ssh", @"mysql", @"postgresql", @"http", @"https", @"redis", @"mongodb", @"rdp", @"socks5", @"generic"];
        NSArray *states = @[@"running", @"starting", @"error", @"stopped"];
        NSBitmapImageRep *preview = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:1120 pixelsHigh:520
            bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
        preview.size = NSMakeSize(560, 260);
        [NSGraphicsContext saveGraphicsState];
        NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:preview];
        for (NSInteger theme = 0; theme < 2; theme++) {
            NSAppearance *appearance = [NSAppearance appearanceNamed:theme ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
            [appearance performAsCurrentDrawingAppearance:^{
                [(theme ? [NSColor colorWithWhite:0.12 alpha:1] : NSColor.whiteColor) setFill];
                NSRectFill(NSMakeRect(theme * 280, 0, 280, 260));
                for (NSInteger row = 0; row < services.count; row++) {
                    [stmServiceName(services[row]) drawAtPoint:NSMakePoint(theme * 280 + 8, 236 - row * 24)
                        withAttributes:@{NSFontAttributeName: [NSFont systemFontOfSize:11], NSForegroundColorAttributeName: NSColor.labelColor}];
                    for (NSInteger col = 0; col < states.count; col++) {
                        NSImage *icon = stmLineImage(states[col], services[row]);
                        assert(!icon.template && icon.size.width == 32 && icon.size.height == 18);
                        [icon drawInRect:NSMakeRect(theme * 280 + 102 + col * 42, 232 - row * 24, 32, 18)];
                    }
                }
            }];
        }
        [NSGraphicsContext restoreGraphicsState];
        if (argc > 1) {
            NSData *png = [preview representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
            assert([png writeToFile:[NSString stringWithUTF8String:argv[1]] atomically:YES]);
        }
        assert([stmServiceName(@"unknown") isEqualToString:@"TCP"]);
        assert(stmLineImage(@"stopped", nil) != nil);
        puts("Service icons: all services and states render in light/dark appearances.");
    }
    return 0;
}
