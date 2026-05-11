using MRMI.Demo.Blazor;
using MRMI.Gateway.Client;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddRazorPages();
builder.Services.AddServerSideBlazor();

// Named clients: "rs" → RS gateway, "ru" → RU gateway
builder.Services.AddHttpClient("rs");
builder.Services.AddHttpClient("ru");
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

app.Run();
