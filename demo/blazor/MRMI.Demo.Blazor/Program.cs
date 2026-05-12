using MRMI.Demo.Blazor;
using MRMI.Gateway.Client;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddRazorPages();
builder.Services.AddServerSideBlazor();
builder.Services.AddSingleton<DemoState>();

var app = builder.Build();

if (!app.Environment.IsDevelopment())
{
    app.UseExceptionHandler("/Error");
    app.UseHsts();
}

app.UseStaticFiles();
app.UseRouting();
app.MapBlazorHub();
app.MapFallbackToPage("/_Host");

// Start SSE background streams immediately so messages arrive as soon as
// any browser tab connects — not just when the Chat page is first opened.
app.Services.GetRequiredService<DemoState>().StartStreaming();

app.Run();
